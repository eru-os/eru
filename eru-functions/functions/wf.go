package functions

import "github.com/eru-os/eru/eru-db/db"

type Workflow struct {
	WfName   string                   `json:"wf_name"`
	WfEvents map[string]WorkflowEvent `json:"wf_events"`
	WfDb     db.DbI                   `json:"-"`
}

type WorkflowEvent struct {
	Function_Name string                   `json:"function_name"`
	FuncGroup     FuncGroup                `json:"function"`
	WfEvents      map[string]WorkflowEvent `json:"wf_events"`
}
