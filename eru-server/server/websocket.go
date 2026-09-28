package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/gorilla/websocket"
)

type WebSocketConfig struct {
	ReadBufferSize  int
	WriteBufferSize int
	MaxConnections  int
	PingInterval    time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	AllowedOrigins  []string
	Subprotocols    []string
}

func DefaultWebSocketConfig() WebSocketConfig {
	return WebSocketConfig{
		ReadBufferSize:  4096,
		WriteBufferSize: 4096,
		MaxConnections:  1000,
		PingInterval:    30 * time.Second,
		ReadTimeout:     60 * time.Second,
		WriteTimeout:    10 * time.Second,
		AllowedOrigins:  []string{"*"},
		Subprotocols:    []string{},
	}
}

type WebSocketConnection struct {
	conn      *websocket.Conn
	writeMux  sync.Mutex
	readMux   sync.Mutex
	closeOnce sync.Once
	closed    chan struct{}
	config    WebSocketConfig
}

func NewWebSocketConnection(conn *websocket.Conn, config WebSocketConfig) *WebSocketConnection {
	wsConn := &WebSocketConnection{
		conn:   conn,
		closed: make(chan struct{}),
		config: config,
	}

	go wsConn.keepAlive()

	return wsConn
}

func (ws *WebSocketConnection) keepAlive() {
	if ws.config.PingInterval <= 0 {
		return
	}

	ticker := time.NewTicker(ws.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			writeDone := make(chan error, 1)
			go func() {
				ws.writeMux.Lock()
				defer ws.writeMux.Unlock()
				writeDone <- ws.conn.WriteMessage(websocket.PingMessage, nil)
			}()

			select {
			case err := <-writeDone:
				if err != nil {
					ws.Close()
					return
				}
			case <-time.After(ws.config.WriteTimeout):
				ws.Close()
				return
			case <-ws.closed:
				return
			}
		case <-ws.closed:
			return
		}
	}
}

func (ws *WebSocketConnection) ReadMessage(ctx context.Context) ([]byte, error) {
	ws.readMux.Lock()
	defer ws.readMux.Unlock()

	if ws.config.ReadTimeout > 0 {
		ws.conn.SetReadDeadline(time.Now().Add(ws.config.ReadTimeout))
	}

	messageType, message, err := ws.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	if messageType == websocket.PongMessage {
		return ws.ReadMessage(ctx)
	}

	if messageType != websocket.TextMessage && messageType != websocket.BinaryMessage {
		return ws.ReadMessage(ctx)
	}

	return message, nil
}

func (ws *WebSocketConnection) WriteMessage(ctx context.Context, data []byte) error {
	ws.writeMux.Lock()
	defer ws.writeMux.Unlock()

	if ws.config.WriteTimeout > 0 {
		ws.conn.SetWriteDeadline(time.Now().Add(ws.config.WriteTimeout))
	}

	return ws.conn.WriteMessage(websocket.TextMessage, data)
}

func (ws *WebSocketConnection) Close() error {
	var err error
	ws.closeOnce.Do(func() {
		close(ws.closed)
		err = ws.conn.Close()
	})
	return err
}

type WebSocketHandler struct {
	config         WebSocketConfig
	upgrader       websocket.Upgrader
	messageHandler func(ctx context.Context, data []byte) ([]byte, error)
	activeConns    sync.Map
}

func NewWebSocketHandler(messageHandler func(ctx context.Context, data []byte) ([]byte, error), config ...WebSocketConfig) *WebSocketHandler {
	var cfg WebSocketConfig
	if len(config) > 0 {
		cfg = config[0]
	} else {
		cfg = DefaultWebSocketConfig()
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  cfg.ReadBufferSize,
		WriteBufferSize: cfg.WriteBufferSize,
		Subprotocols:    cfg.Subprotocols,
		CheckOrigin: func(r *http.Request) bool {
			if len(cfg.AllowedOrigins) == 0 {
				return true
			}

			origin := r.Header.Get("Origin")
			for _, allowed := range cfg.AllowedOrigins {
				if allowed == "*" || allowed == origin {
					return true
				}
			}
			return false
		},
	}

	return &WebSocketHandler{
		config:         cfg,
		upgrader:       upgrader,
		messageHandler: messageHandler,
	}
}

func (h *WebSocketHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		logs.WithContext(ctx).Error("WebSocket upgrade failed: " + err.Error())
		return
	}

	connID := r.RemoteAddr + "-" + time.Now().String()

	count := h.GetActiveConnections()
	if count >= h.config.MaxConnections {
		conn.Close()
		logs.WithContext(ctx).Warn("Max WebSocket connections reached")
		return
	}

	h.activeConns.Store(connID, conn)
	defer h.activeConns.Delete(connID)

	wsConn := NewWebSocketConnection(conn, h.config)
	defer wsConn.Close()

	logs.WithContext(ctx).Info("WebSocket connection established")

	conn.SetPongHandler(func(appData string) error {
		return nil
	})

	logs.WithContext(ctx).Info(fmt.Sprintf("WebSocket headers: %v", r.Header))

	for {
		logs.WithContext(ctx).Info("WebSocket reading message")
		select {
		case <-ctx.Done():
			return
		case <-wsConn.closed:
			return
		default:
			message, err := wsConn.ReadMessage(ctx)
			logs.WithContext(ctx).Info("WebSocket received message: " + string(message))
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logs.WithContext(ctx).Error("WebSocket read error: " + err.Error())
				}
				return
			}

			if len(message) == 0 {
				continue
			}

			// Log raw message for debugging
			logs.WithContext(ctx).Info("WebSocket received message: " + string(message))

			response, err := h.messageHandler(ctx, message)
			if err != nil {
				logs.WithContext(ctx).Error("WebSocket message handler error: " + err.Error())
				continue
			}

			if response != nil {
				logs.WithContext(ctx).Info("WebSocket sending response: " + string(response))
				err = wsConn.WriteMessage(ctx, response)
				if err != nil {
					logs.WithContext(ctx).Error("WebSocket write error: " + err.Error())
					return
				}
			} else {
				logs.WithContext(ctx).Info("WebSocket handler returned nil response")
			}
		}
	}
}

func (h *WebSocketHandler) GetActiveConnections() int {
	count := 0
	h.activeConns.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

func (h *WebSocketHandler) CloseAllConnections() {
	h.activeConns.Range(func(key, value interface{}) bool {
		if conn, ok := value.(*websocket.Conn); ok {
			conn.Close()
		}
		return true
	})
}

func (h *WebSocketHandler) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"active_connections": h.GetActiveConnections(),
		"max_connections":    h.config.MaxConnections,
		"ping_interval":      h.config.PingInterval.String(),
		"read_timeout":       h.config.ReadTimeout.String(),
		"write_timeout":      h.config.WriteTimeout.String(),
	}
}
