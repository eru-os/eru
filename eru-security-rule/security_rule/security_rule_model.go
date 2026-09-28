package security_rule

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	"github.com/eru-os/eru/eru-templates/gotemplate"
	eru_utils "github.com/eru-os/eru/eru-utils"
)

const (
	MAKE_JSON_ARRAY_FN     = "$make_json_array_fn"
	MAKE_JSON_ARRAY_FN_STR = "$make_json_array_fn_str"
)

type CustomRule struct {
	AND []CustomRuleDetails `json:"and,omitempty"`
	OR  []CustomRuleDetails `json:"or,omitempty"`
}

type CustomRuleDetails struct {
	DataType  string              `json:"data_type,omitempty"`
	Variable1 string              `json:"variable1,omitempty"`
	Variable2 string              `json:"variable2,omitempty"`
	Operator  string              `json:"operator,omitempty"`
	ErrorMsg  string              `json:"error_msg,omitempty"`
	AND       []CustomRuleDetails `json:"and,omitempty"`
	OR        []CustomRuleDetails `json:"or,omitempty"`
	Template  string              `json:"template,omitempty"`
}

type SecurityRule struct {
	RuleType     string                 `json:"rule_type"`
	CustomRule   CustomRule             `json:"custom_rule"`
	IgnoreFilter map[string]interface{} `json:"ignore_filter"`
	IgnoreRecord string                 `json:"ignore_record"`
}

func (sr SecurityRule) Stringify(ctx context.Context, vars map[string]interface{}, ignoreIfNotFound bool, mainTableName string, ctjMap map[string]string) (str string, templates []string, ptables []string, err error) {
	logs.WithContext(ctx).Debug("Stringify - Start")
	if len(sr.CustomRule.AND) > 0 {
		str, templates, ptables, err = processRuleClause(ctx, sr.CustomRule.AND, "and", vars, ignoreIfNotFound, mainTableName, ctjMap)
		return
	}
	if len(sr.CustomRule.OR) > 0 {
		str, templates, ptables, err = processRuleClause(ctx, sr.CustomRule.OR, "or", vars, ignoreIfNotFound, mainTableName, ctjMap)
		return
	}
	return
}
func processRuleClause(ctx context.Context, rules []CustomRuleDetails, conditionType string, vars map[string]interface{}, ignoreIfNotFound bool, mainTableName string, ctjMap map[string]string) (ruleOutput string, templates []string, ptables []string, err error) {
	logs.WithContext(ctx).Debug("processRuleClause - Start")
	var strArray []string
	var tmpTemplates []string
	str := ""
	for _, v := range rules {
		if len(v.AND) > 0 {
			str, ptables, tmpTemplates, err = processRuleClause(ctx, v.AND, "and", vars, ignoreIfNotFound, mainTableName, ctjMap)
			templates = append(templates, tmpTemplates...)
		} else if len(v.OR) > 0 {
			str, ptables, tmpTemplates, err = processRuleClause(ctx, v.OR, "or", vars, ignoreIfNotFound, mainTableName, ctjMap)
			templates = append(templates, tmpTemplates...)
		} else if v.Template != "" {
			str = fmt.Sprint("$TEMPLATE_", v.Template)
			templates = append(templates, v.Template)
		} else {
			str, ptables, err = stringifyRule(ctx, v, conditionType, vars, ignoreIfNotFound, mainTableName, ctjMap)
		}
		if str != "" {
			strArray = append(strArray, str)
		}
	}
	if len(strArray) > 0 {
		conditionType = fmt.Sprint(" ", conditionType, " ")
		ruleOutput = fmt.Sprint("( ", strings.Join(strArray, conditionType), " )")
	}
	return
}

func stringifyRule(ctx context.Context, cd CustomRuleDetails, conditionType string, vars map[string]interface{}, ignoreIfNotFound bool, mainTableName string, ctjMap map[string]string) (str string, ptables []string, err error) {
	logs.WithContext(ctx).Debug("stringifyRule - Start")
	op := ""
	valPrefix := ""
	valSuffix := ""
	if cd.DataType == "string" {
		valPrefix = "'"
		valSuffix = "'"
	}
	if cd.Operator == "like" {
		valPrefix = "'%"
		valSuffix = "%'"
	}
	switch cd.Operator {
	case "btw":
		op = " between "
		break
	case "gte":
		op = " >= "
		break
	case "lte":
		op = " <= "
		break
	case "gt":
		op = " > "
		break
	case "lt":
		op = " < "
		break
	case "eq":
		op = " = "
		break
	case "ne":
		op = " <> "
		break
	case "in":
		op = " in "
		break
	case "nin":
		op = " not in "
		break
	case "like":
		op = " like "
		break
	case "nlike":
		op = " not like "
		break
	case "eq_null":
		op = " is null "
		break
	case "neq_null":
		op = " is not null "
		break
	case "jin":
		op = MAKE_JSON_ARRAY_FN
		break
	case "jnin":
		op = MAKE_JSON_ARRAY_FN
		break
	case "ex_in":
		op = " exists "
		break
	case "nex_in":
		op = " not exists "
		break
	case "ex_jin":
		op = " exists "
		break
	case "nex_jin":
		op = " not exists "
		break
	case "ex_pj":
		op = " exists "
		break
	case "nex_pj":
		op = " not exists "
		break
	default:
		//do nothing
		break
	}
	cdv1 := cd.Variable1
	if !(strings.Contains(cd.Variable1, ".")) {
		cdv1 = fmt.Sprint(mainTableName, ".", cdv1)
	}
	cdv2 := cd.Variable2
	var1Bytes, err := processTemplate(ctx, "customrule", cdv1, vars, "string")
	if err == nil {
		cdv1 = fmt.Sprint(valPrefix, string(var1Bytes), valSuffix)
	} else if ignoreIfNotFound && err.Error() != "no variable prefix found" {
		return "", nil, nil
	}
	var2Bytes, err := processTemplate(ctx, "customrule", cdv2, vars, "string")
	if err == nil {
		cdv2 = fmt.Sprint(valPrefix, string(var2Bytes), valSuffix)
	} else if ignoreIfNotFound && err.Error() != "no variable prefix found" {
		return "", nil, nil
	}
	existsClause := false
	existsOp := ""
	if cd.Operator == "ex_in" || cd.Operator == "nex_in" || cd.Operator == "ex_pj" || cd.Operator == "nex_pj" {
		existsClause = true
		existsOp = " in "
	} else if cd.Operator == "ex_jin" || cd.Operator == "nex_jin" {
		existsClause = true
		existsOp = " ?| array " //TODO - make it DB specific
	}
	if existsClause {
		exStr := ""
		for k, v := range ctjMap {
			if strings.Contains(cdv1, k) {
				if cd.Operator == "ex_jin" || cd.Operator == "nex_jin" || cd.Operator == "ex_in" || cd.Operator == "nex_in" {
					exStr = fmt.Sprint("select 1 from (select * from ", k, " where ", cdv1, existsOp, cdv2, ") x where ", strings.Replace(v, k, "x", -1))
				}
				if cd.Operator == "ex_pj" || cd.Operator == "nex_pj" {
					exStr = fmt.Sprint("select 1 from (select * from $", k, "$ ", ") x where ", strings.Replace(v, k, "x", -1))
					ptables = append(ptables, k)
				}
				return fmt.Sprint(op, " (", exStr, ")"), ptables, nil
			}
		}
	}
	return fmt.Sprint(cdv1, op, cdv2), ptables, nil
}

func processTemplate(ctx context.Context, templateName string, templateString string, vars map[string]interface{}, outputType string) (output []byte, err error) {
	logs.WithContext(ctx).Debug("processTemplate - Start")
	ruleValue := strings.SplitN(templateString, ".", 2)
	if ruleValue[0] == "token" {
		templateStr := ruleValue[1]
		if !(strings.HasPrefix(ruleValue[1], "{{")) {
			templateStr = fmt.Sprint("{{ .", ruleValue[1], " }}")
		}
		templateStr = eru_utils.ReplaceVariables(ctx, templateStr, vars)
		goTmpl := gotemplate.GoTemplate{Name: templateName, Template: templateStr}
		outputObj, err := goTmpl.Execute(ctx, vars["token"], outputType)
		if err != nil {
			err = logs.Err(ctx, err, "")
			return nil, err
		} else if outputType == "string" {
			return []byte(outputObj.(string)), nil
		} else {
			output, err = json.Marshal(outputObj)
			if err != nil {
				err = logs.Err(ctx, err, "")
				return nil, err
			}
		}
	} else {
		err = logs.Err(ctx, errors.New("no variable prefix found"), "")
	}
	//todo - to add if prefix is not token
	return
}
