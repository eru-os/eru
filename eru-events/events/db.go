package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/eru-os/eru/eru-db/db"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_models "github.com/eru-os/eru/eru-models"
	eru_utils "github.com/eru-os/eru/eru-utils"
	"github.com/jmoiron/sqlx"
)

type DB_Event struct {
	Event
	MsgToPoll int32  `json:"msg_to_poll" eru:"required"`
	EventDb   db.DbI `json:"-"`
	//Con       *sqlx.DB `json:"-"`
	//ConStatus bool     `json:"-"`
}

const (
	SELECT_EVENT_MSG = "update erufunctions_async_loop x set async_status='PENDING' from (select a.async_loop_id, a.async_id, a.event_id from erufunctions_async_loop a inner join erufunctions_async b on a.event_id=b.event_id and b.async_event_name = ??? where a.async_status = 'TORUN' limit $LIMIT for update of a skip locked) y where x.async_loop_id=y.async_loop_id returning y.async_id, y.event_id"
	// INSERT_ASYNC_EVENT and INSERT_ASYNC_LOOP mirror the inserts the db side procedure does
	// when it raises an async event - they are kept here so the go side and the procedure stay
	// in step. project_id and tenant_id say who the function has to be resolved for when the
	// row is picked up; tenant_id is the route form, so "<default tenant>___<tenant>" when a
	// default tenant applies, and empty for a project level call.
	INSERT_ASYNC_EVENT = "INSERT INTO erufunctions_async(event_id, func_group_name,func_step_name,event_msg,event_request,request_id,async_event_name,project_id,tenant_id) VALUES (event_id, function_name,'',event_msg::jsonb,'','',event_name,project_id,tenant_id);"
	INSERT_ASYNC_LOOP  = "INSERT INTO erufunctions_async_loop(event_id,async_id, loop_var, async_status) VALUES (event_id, async_id,'{}'::jsonb,'TORUN');"
)

func (dbEvent *DB_Event) Init(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Init - Start")

	return err
}

func (dbEvent *DB_Event) CreateEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("CreateEvent - Start")

	return
}

func (dbEvent *DB_Event) DeleteEvent(ctx context.Context) (err error) {
	logs.WithContext(ctx).Info("DeleteEvent - Start")

	return
}

func (dbEvent *DB_Event) MakeFromJson(ctx context.Context, rj *json.RawMessage) error {
	logs.WithContext(ctx).Debug("MakeFromJson - Start")
	err := json.Unmarshal(*rj, &dbEvent)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return err
	}
	return nil
}

func (dbEvent *DB_Event) Publish(ctx context.Context, msg interface{}, e EventI) (msgId string, err error) {
	logs.WithContext(ctx).Debug("Publish - Start")

	return
}

func (dbEvent *DB_Event) Poll(ctx context.Context) (eventMsgs []EventMsg, err error) {
	logs.WithContext(ctx).Debug("Poll - Start")

	var dbEventQueries []*eru_models.Queries
	dbEventQuery := eru_models.Queries{}
	//dbEvent.MsgToPoll
	dbEventQuery.Query = dbEvent.EventDb.GetDbQuery(ctx, strings.Replace(SELECT_EVENT_MSG, "$LIMIT", fmt.Sprint(2), 1))
	dbEventQuery.Vals = append(dbEventQuery.Vals, dbEvent.EventName)
	dbEventQuery.Rank = 1
	dbEventQueries = append(dbEventQueries, &dbEventQuery)
	dbEventOutput, err := eru_utils.ExecuteDbSave(ctx, dbEvent.EventDb.GetConn(), dbEventQueries)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return nil, err
	}
	logs.WithContext(ctx).Info(fmt.Sprint("dbEventOutput = ", dbEventOutput))
	for _, messageRow := range dbEventOutput {
		for _, message := range messageRow {
			var async_id interface{}
			var event_id interface{}
			async_idOk := false
			async_id, async_idOk = message["async_id"]
			if !async_idOk {
				logs.WithContext(ctx).Error("async_id not found")
				return
			}
			event_idOk := false
			event_id, event_idOk = message["event_id"]
			if !event_idOk {
				logs.WithContext(ctx).Info("event_id not found")
				// let event id be blank and continue
			}

			if async_id != nil {
				eId := ""
				if event_id != nil {
					eId = event_id.(string)
				}
				eventMsg := EventMsg{Msg: async_id.(string), MsgIdentifer: eId}
				eventMsgs = append(eventMsgs, eventMsg)
			}
		}
	}
	return
}

func (dbEvent *DB_Event) DeleteMessage(ctx context.Context, msgIdentifier string) (err error) {
	logs.WithContext(ctx).Info(fmt.Sprint("DeleteMessage"))

	return
}

func (dbEvent *DB_Event) Clone(ctx context.Context) (cloneEvent EventI, err error) {
	cloneEventI, cloneEventIErr := eru_utils.CloneInterface(ctx, dbEvent)
	if cloneEventIErr != nil {
		err = cloneEventIErr
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	cloneEventOk := false
	cloneEvent, cloneEventOk = cloneEventI.(*DB_Event)
	if !cloneEventOk {
		err = errors.New("event cloning failed")
		logs.WithContext(ctx).Error(err.Error())
		return
	}
	return
}
func (dbEvent *DB_Event) SetCon(con *sqlx.DB, dbType string) {
	dbEvent.EventDb = db.GetDb(dbType)
	dbEvent.EventDb.SetConn(con)
	//dbEvent.Con = con
	//dbEvent.ConStatus = true
}
