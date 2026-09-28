package sm

type GcpSmStore struct {
	SmStore
	SmName string `json:"sm_name" eru:"required"`
}
