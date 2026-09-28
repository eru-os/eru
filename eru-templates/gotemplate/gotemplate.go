package gotemplate

import (
	"bytes"
	"context"
	b64 "encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"
	"time"

	sprig "github.com/Masterminds/sprig/v3"
	eruaes "github.com/eru-os/eru/eru-crypto/aes"
	eruhmac "github.com/eru-os/eru/eru-crypto/hmac"
	"github.com/eru-os/eru/eru-crypto/jwt"
	erumd5 "github.com/eru-os/eru/eru-crypto/md5"
	erursa "github.com/eru-os/eru/eru-crypto/rsa"
	erusha "github.com/eru-os/eru/eru-crypto/sha"
	logs "github.com/eru-os/eru/eru-logs/eru-logs"
	eru_writes "github.com/eru-os/eru/eru-read-write/eru_writes"
	"github.com/eru-os/eru/eru-secret-manager/kms"
	eruutils "github.com/eru-os/eru/eru-utils"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

type GoTemplate struct {
	Name     string
	Template string
}

type OrderedMap struct {
	Rank float64
	Obj  map[string]interface{}
}

type sorter []*OrderedMap

func (a sorter) Len() int {
	return len(a)
}
func (a sorter) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a sorter) Less(i, j int) bool {
	return a[i].Rank < a[j].Rank
}
func GenericFuncMap(ctx context.Context) map[string]interface{} {
	return map[string]interface{}{
		"inc": func(n int) int {
			return n + 1
		},
		"marshalJSON": func(j interface{}) ([]byte, error) {
			d, err := json.Marshal(j)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
			return d, err
		},
		"unmarshalJSON": func(b []byte) (d interface{}, err error) {
			err = json.Unmarshal(b, &d)
			return
		},
		"b64Encode": func(str []byte) (string, error) {
			return b64.StdEncoding.EncodeToString(str), nil
		},
		"b64Decode": func(str string) (string, error) {
			decodeBytes, err := b64.StdEncoding.DecodeString(str)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				//return empty string with nil error to silently proceed even if base64 conversion fails
				return "", nil
			}
			return string(decodeBytes), nil
		},
		"hexEncode": func(str []byte) string {
			return hex.EncodeToString(str)
		},
		"hexDecode": func(str string) (string, error) {
			decodeBytes, err := hex.DecodeString(str)
			return string(decodeBytes), err
		},
		"len": func(j interface{}) (int, error) {
			strJ, err := json.Marshal(j)
			return len(strJ), err
		},
		"aesEncryptGCM": func(pb []byte, k []byte) ([]byte, error) {
			dst, err := eruaes.Encrypt(ctx, pb, k)
			return dst, err
		},
		"aesDecryptGCM": func(eb []byte, k []byte) ([]byte, error) {
			return eruaes.Decrypt(ctx, eb, k)
		},
		"aesEncryptECB": func(pb []byte, k []byte) ([]byte, error) {
			dst, err := eruaes.EncryptECB(ctx, pb, k)
			return dst, err
		},
		"aesDecryptECB": func(eb []byte, k []byte) ([]byte, error) {
			return eruaes.DecryptECB(ctx, eb, k)
		},
		"aesEncryptCBC": func(pb []byte, k []byte, iv []byte) ([]byte, error) {
			dst, err := eruaes.EncryptCBC(ctx, pb, k, iv)
			return []byte(dst), err
		},
		"aesDecryptCBC": func(eb []byte, k []byte, iv []byte) ([]byte, error) {
			return eruaes.DecryptCBC(ctx, eb, k, iv)
		},
		"encryptRSACert": func(j []byte, pubK string) ([]byte, error) {
			return erursa.EncryptWithCert(ctx, j, pubK)
		},
		"bytesToString": func(b []byte) string {
			return string(b)
		},
		"stringToByte": func(s string) []byte {
			return []byte(s)
		},
		"stringify": func(j interface{}) (string, error) {
			d, err := json.Marshal(j)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
			}
			return string(d), err
		},
		"unquote": func(s string) string {
			str, uerr := strconv.Unquote(string(s))
			if uerr != nil {
				logs.WithContext(ctx).Error(uerr.Error())
			}
			return str
		},
		"doubleQuote": func(s string) string {
			return strconv.Quote(fmt.Sprint("\"", s, "\""))
		},
		"generateAesKey": func(bits int) ([]byte, error) {
			aesObj, err := eruaes.GenerateKey(ctx, bits)
			if err != nil {
				return nil, err
			}
			return aesObj.Key, nil
		},
		"generate_rsa_keypair": func(bits int) (interface{}, error) {
			rsaObj, err := erursa.GenerateKeyPair(ctx, bits)
			if err != nil {
				return nil, err
			}
			return rsaObj, nil
		},
		"hmac": func(b string, secret string) []byte {
			return eruhmac.Hmac([]byte(b), secret)
		},
		"shaHash": func(b string, bits int) (string, error) {
			switch bits {
			case 256:
				return hex.EncodeToString(erusha.NewSHA256([]byte(b))), nil
			case 512:
				return hex.EncodeToString(erusha.NewSHA512([]byte(b))), nil
			default:
				err := errors.New(fmt.Sprint("SHA function not defined for ", bits, "bits"))
				logs.WithContext(ctx).Error(err.Error())
				return "", err
			}
		},
		"md5": func(str string, output string) (string, error) {
			return erumd5.Md5(ctx, str, output)
		},
		"PKCS7Pad": func(buf []byte, size int) []byte {
			return eruaes.Pad(buf, size)
		},
		"PKCS7Unpad": func(buf []byte) ([]byte, error) {
			return eruaes.Unpad(buf)
		},
		"saveVar": func(vars map[string]interface{}, ketToSave string, valueToSave interface{}) error {
			if vars == nil {
				vars = make(map[string]interface{})
			}
			vars[ketToSave] = valueToSave
			return nil
		},
		"concatMapKeyVal": func(vars map[string]interface{}, keys []string, seprator string) string {
			str := ""
			for _, k := range keys {
				str = fmt.Sprint(str, k, "=", vars[k], "|")
			}
			return str
		},
		"concatMapKeyValUnordered": func(vars map[string]interface{}, seprator string, keyFirst bool, varSeprator string) string {
			str := ""
			for k, _ := range vars {
				if keyFirst {
					str = fmt.Sprint(str, k, seprator, vars[k], varSeprator)
				} else {
					str = fmt.Sprint(str, vars[k], seprator, k, varSeprator)
				}

			}
			return str
		},
		"makeMapKeyValUnordered": func(str string, seprator string) (vars map[string]interface{}) {
			vars = make(map[string]interface{})
			tmpStr := strings.Split(str, seprator)
			for _, v := range tmpStr {
				vSplit := strings.Split(v, "=")
				splitStr := ""
				if len(vSplit) == 2 {
					splitStr = vSplit[1]
				}
				vars[vSplit[0]] = splitStr
			}
			return vars
		},
		"overwriteMap": func(orgMap map[string]interface{}, b []byte) (d interface{}, err error) {
			newMap := make(map[string]interface{})
			err = json.Unmarshal(b, &newMap)
			for k, v := range newMap {
				orgMap[k] = v
			}
			d, err = json.Marshal(orgMap)
			return
		},
		"removeMapKey": func(orgMap map[string]interface{}, key string) (d map[string]interface{}, err error) {
			v, err := json.Marshal(orgMap)
			if err != nil {
				return orgMap, err
			}
			newMap := make(map[string]interface{})
			err = json.Unmarshal(v, &newMap)
			if err != nil {
				return orgMap, err
			}
			delete(newMap, key)
			return newMap, nil
		},
		"getMapValue": func(orgMap map[string]interface{}, key string) (d interface{}, err error) {
			d = make(map[string]interface{})
			ok := false
			d, ok = orgMap[key]
			if !ok {
				return orgMap, err
			}
			return d, nil
		},
		"getMapKeys": func(orgMap map[string]interface{}) (d []string, err error) {
			for k, _ := range orgMap {
				d = append(d, k)
			}
			return d, nil
		},
		"arrayLen": func(arr interface{}) (d int, err error) {
			d = 0
			if o, oOk := arr.([]interface{}); oOk {
				return len(o), err
			}
			if o, oOk := arr.([]string); oOk {
				return len(o), err
			}
			return d, errors.New("not an array")
		},
		"is_array": func(arr interface{}) (r bool) {
			if _, oOk := arr.([]interface{}); oOk {
				return true
			}
			return false
		},
		"getMapPointerValue": func(orgMap map[string]*interface{}, key string) (d interface{}, err error) {
			d = make(map[string]interface{})
			ok := false
			d, ok = orgMap[key]
			if !ok {
				return orgMap, err
			}
			return d, nil
		},
		"getArrayValue": func(orgArray []interface{}, index int, emptyValue interface{}) (d interface{}) {
			if emptyValue == "object" {
				d = make(map[string]interface{})
			} else if emptyValue == "string" {
				d = ""
			} else if emptyValue == "number" {
				d = 0
			}
			if len(orgArray) <= index {
				return d
			}
			d = orgArray[index]
			return d
		},
		"sortMapArray": func(mapArray []interface{}, sortKey string) (mapArraySorted []interface{}, err error) {
			var tmpArray []*OrderedMap
			for _, v := range mapArray {
				if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
					if r, rOk := vMap[sortKey].(float64); !rOk {
						err = errors.New("sortKey is not an int")
						return
					} else {
						o := OrderedMap{
							Rank: r,
							Obj:  vMap,
						}
						tmpArray = append(tmpArray, &o)
					}
				} else {
					err = errors.New("not a map array")
					return
				}
			}
			sort.Sort(sorter(tmpArray))
			for _, nv := range tmpArray {
				mapArraySorted = append(mapArraySorted, nv.Obj)
			}
			return
		},
		"logobject": func(v interface{}) (err error) {
			vobj, err := json.Marshal(v)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return
			}
			logs.WithContext(ctx).Info(fmt.Sprint("logobject = ", string(vobj)))
			return
		},
		"logstring": func(str interface{}) (err error) {
			logs.WithContext(ctx).Info(fmt.Sprint("logstring = ", str))
			return
		},
		"logerror": func(str interface{}) (err error) {
			logs.WithContext(ctx).Error(fmt.Sprint("logstring = ", str))
			return
		},
		"uuid": func() (uuidStr string, err error) {
			uuidStr = uuid.New().String()
			return
		},
		"current_date": func() (dt string, err error) {
			dt = time.Now().Format("2006-01-02")
			return
		},
		"date_diff": func(indtstr string, n int, t string) (dt string, err error) {
			indt, err := time.Parse("2006-01-02", indtstr)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				err = errors.New("Invalid date format - expected formatted 2006-01-02")
				logs.WithContext(ctx).Error(err.Error())
				return "", err
			}
			y := 0
			m := 0
			d := 0
			switch t {
			case "days":
				d = n
			case "months":
				m = n
			case "years":
				y = n
			default:
				err = errors.New("Invalid type - expected values are years months and days")
				logs.WithContext(ctx).Error(err.Error())
				return "", err
			}
			dt = indt.AddDate(y, m, d).Format("2006-01-02")
			return dt, nil
		},
		"date_part": func(dtStr string, dtPart string) (datePart string, err error) {
			dt, err := time.Parse("2006-01-02", dtStr)
			switch dtPart {
			case "DAY":
				dStr := strconv.Itoa(dt.Day())
				datePart = strings.Repeat("0", 2-len(dStr)) + dStr
			case "MONTHN":
				mStr := strconv.Itoa(int(dt.Month()))
				datePart = strings.Repeat("0", 2-len(mStr)) + mStr
			case "MONTH":
				datePart = dt.Month().String()
			case "YEAR":
				datePart = strconv.Itoa(dt.Year())
			default:
				datePart = ""
			}
			return
		},
		"date_format": func(dtStr string, srcLayout string, newLayout string) (datePart string, err error) {
			vDate, vErr := time.Parse(srcLayout, dtStr)
			if vErr != nil {
				err = vErr
				return
			}
			datePart = vDate.Format(newLayout)
			return
		},
		"str_concat": func(sep string, inStr ...string) (str string, err error) {
			str = strings.Join(inStr, sep)
			return
		},
		"str_replace": func(txt string, oldStr string, newStr string, num int) (str string, err error) {
			str = strings.Replace(txt, oldStr, newStr, num)
			return
		},
		"removenull": func(txt string) (str string, err error) {
			str = strings.Replace(txt, "\u0000", "", -1)
			str = strings.Replace(str, "\\u0000", "", -1)
			return
		},
		"math_add": func(args ...interface{}) (result float64, err error) {
			num := 0.0
			for _, a := range args {
				switch v := a.(type) {
				case int, float64:
					_ = v
					num, err = strconv.ParseFloat(fmt.Sprintf("%v", a), 64)
					if err != nil {
						logs.WithContext(ctx).Error(err.Error())
						return
					}
					result = result + num
				default:
					err = errors.New("Non Numeric Input")
					return
				}
			}
			return result, nil
		},
		"math_sub": func(a interface{}, b interface{}) (result float64, err error) {
			var n1, n2 float64
			switch v := a.(type) {
			case int, float64:
				_ = v
				n1, err = strconv.ParseFloat(fmt.Sprintf("%v", a), 64)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return
				}
			default:
				err = errors.New("Non Numeric Input")
				return
			}
			switch v := b.(type) {
			case int, float64:
				_ = v
				n2, err = strconv.ParseFloat(fmt.Sprintf("%v", b), 64)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return
				}
			default:
				err = errors.New("Non Numeric Input")
				return
			}
			result = n1 - n2
			return result, nil
		},
		"math_div": func(a interface{}, b interface{}) (result float64, err error) {
			var n1, n2 float64
			switch v := a.(type) {
			case int, float64:
				_ = v
				n1, err = strconv.ParseFloat(fmt.Sprintf("%v", a), 64)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return
				}
			default:
				err = errors.New("Non Numeric Input")
				return
			}
			switch v := b.(type) {
			case int, float64:
				_ = v
				n2, err = strconv.ParseFloat(fmt.Sprintf("%v", b), 64)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return
				}
			default:
				err = errors.New("Non Numeric Input")
				return
			}
			result = n1 / n2
			return result, nil
		},
		"math_mul": func(a float64, b float64) (result float64) {
			return a * b
		},
		"math_round": func(a interface{}, r float64) (result float64, err error) {
			var n1 float64
			m := 1.0
			switch v := a.(type) {
			case int, float64:
				_ = v
				n1, err = strconv.ParseFloat(fmt.Sprintf("%v", a), 64)
				if err != nil {
					logs.WithContext(ctx).Error(err.Error())
					return
				}
			default:
				err = errors.New("Non Numeric Input")
				return
			}
			result = math.Round(n1*m*r) / (m * r)
			return result, nil
		},
		"excelToJson": func(fData string, sheetNames string, firstRowHeader string, headers string, keys string) (fJson interface{}, err error) {
			return excelToJson(ctx, fData, sheetNames, firstRowHeader, headers, keys)
		},
		"null": func() interface{} {
			return nil
		},
		"char_index": func(s string, c string) int {
			return strings.Index(s, c)
		},
		"new_jwt": func(privateKeyStr string, claimsMap map[string]interface{}) (tokenString string, err error) {
			return jwt.CreateJWT(ctx, privateKeyStr, claimsMap, nil)
		},
		"jwtClaims": func(token string, jwkUrl string) (claims interface{}, err error) {
			claims, err = jwt.DecryptTokenJWK(ctx, token, jwkUrl)
			if err != nil {
				logs.WithContext(ctx).Error(err.Error())
				return nil, nil
			}
			return
		},
		"evalFilter": func(filter map[string]interface{}, record map[string]interface{}) (result bool, err error) {
			return EvalFilter(ctx, filter, record)
		},
		"makeFilter": func(filter string, jsonKey string) (silterStr string, err error) {
			return makeFilter(ctx, filter, jsonKey)
		},
		"makeParentFilter": func(filter string, jsonKey string, parentPrefix string) (silterStr string, err error) {
			return makeParentFilter(ctx, filter, jsonKey, parentPrefix)
		},
		"makeFilterV2": func(filter string, jsonKey string, parentPrefix string, parentNames string, defaultPrefix string) (silterStr string, err error) {
			return makeFilterV2(ctx, filter, jsonKey, parentPrefix, parentNames, defaultPrefix)
		},
		"fetch_filter_keys": func(filter string, parentPrefix string) (filterKeys []string, err error) {
			return fetchFilterKeys(ctx, filter, parentPrefix)
		},
		"execTemplate": func(obj interface{}, templateString string, outputFormat string) (output interface{}, err error) {
			goTmpl := GoTemplate{"subtemplate", templateString}
			return goTmpl.Execute(ctx, obj, outputFormat)
		},
		"kmsDecrypt": func(eStr []byte, kmsStoreType string, region string, kmsId string, kmsAlias string) (output string, err error) {
			ksMap := kms.KmsStore{KmsStoreType: kmsStoreType}
			kmsMap := kms.AwsKmsStore{KmsName: kmsId, KmsAlias: kmsAlias, Region: region, KmsStore: ksMap}
			plainBytes, err := kmsMap.Decrypt(ctx, eStr)
			return string(plainBytes), err
		},
		"jsonToCsvB64": func(mapObjs []interface{}, hasHeader bool) (csvStr string, err error) {
			cwd := eru_writes.CsvWriteData{}
			csvBytes, csvErr := cwd.MapToCsv(ctx, mapObjs, hasHeader)
			if csvErr != nil {
				logs.WithContext(ctx).Error(csvErr.Error())
				return "", csvErr
			}
			return b64.StdEncoding.EncodeToString(csvBytes), nil
		},
		"jsonToCsv": func(mapObjs []interface{}, hasHeader bool) (csvStr string, err error) {
			cwd := eru_writes.CsvWriteData{}
			csvBytes, csvErr := cwd.MapToCsv(ctx, mapObjs, hasHeader)
			if csvErr != nil {
				logs.WithContext(ctx).Error(csvErr.Error())
				return "", csvErr
			}
			return string(csvBytes), nil
		},
		"getObjDiff": func(a map[string]interface{}, b map[string]interface{}) (r map[string]interface{}, err error) {
			r = make(map[string]interface{})
			o := make(map[string]interface{})
			n := make(map[string]interface{})
			var oDiffR eruutils.DiffReporter
			for k, v := range a {
				if b[k] != nil {
					if !cmp.Equal(v, b[k], cmp.Reporter(&oDiffR)) {
						o[k] = v
						n[k] = b[k]
					}
				}
			}
			r["o"] = o
			r["n"] = n

			return r, err
		},
	}
}

func (goTmpl *GoTemplate) Validate(ctx context.Context) (err error) {
	logs.WithContext(ctx).Debug("Validate - Start")
	t := template.New(goTmpl.Name).Funcs(sprig.FuncMap()).Funcs(GenericFuncMap(ctx))
	_, err = t.Parse(strings.ReplaceAll(goTmpl.Template, "\n", ""))
	return err
}

// FieldReferences returns the identifier chain of every field referenced by the
// template, e.g. {{.ResVars.step1.Body}} yields ["ResVars", "step1", "Body"].
// Used to check that a template only reads variables that will actually exist.
func (goTmpl *GoTemplate) FieldReferences(ctx context.Context) (refs [][]string, err error) {
	logs.WithContext(ctx).Debug("FieldReferences - Start")
	t := template.New(goTmpl.Name).Funcs(sprig.FuncMap()).Funcs(GenericFuncMap(ctx))
	t, err = t.Parse(strings.ReplaceAll(goTmpl.Template, "\n", ""))
	if err != nil {
		return nil, err
	}
	for _, tmpl := range t.Templates() {
		if tmpl.Tree == nil {
			continue
		}
		collectFieldReferences(tmpl.Tree.Root, &refs)
	}
	return refs, nil
}

func collectFieldReferences(node parse.Node, refs *[][]string) {
	switch n := node.(type) {
	case nil:
		return
	case *parse.FieldNode:
		if len(n.Ident) > 0 {
			*refs = append(*refs, n.Ident)
		}
	case *parse.ChainNode:
		collectFieldReferences(n.Node, refs)
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			collectFieldReferences(child, refs)
		}
	case *parse.ActionNode:
		collectFieldReferences(n.Pipe, refs)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, cmd := range n.Cmds {
			collectFieldReferences(cmd, refs)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			collectFieldReferences(arg, refs)
		}
	case *parse.IfNode:
		collectBranchFieldReferences(n.BranchNode, refs)
	case *parse.RangeNode:
		collectBranchFieldReferences(n.BranchNode, refs)
	case *parse.WithNode:
		collectBranchFieldReferences(n.BranchNode, refs)
	case *parse.TemplateNode:
		collectFieldReferences(n.Pipe, refs)
	}
}

func collectBranchFieldReferences(branch parse.BranchNode, refs *[][]string) {
	collectFieldReferences(branch.Pipe, refs)
	if branch.List != nil {
		collectFieldReferences(branch.List, refs)
	}
	if branch.ElseList != nil {
		collectFieldReferences(branch.ElseList, refs)
	}
}

type TemplateDict struct {
	Keys     []string
	Dynamic  bool
	Children map[string]*TemplateDict
}

func (td *TemplateDict) HasKey(key string) bool {
	if td == nil {
		return false
	}
	for _, k := range td.Keys {
		if k == key {
			return true
		}
	}
	return false
}

func (td *TemplateDict) Child(key string) *TemplateDict {
	if td == nil || td.Children == nil {
		return nil
	}
	return td.Children[key]
}

func (goTmpl *GoTemplate) RootDict(ctx context.Context) (rootDict *TemplateDict, err error) {
	logs.WithContext(ctx).Debug("RootDict - Start")
	t := template.New(goTmpl.Name).Funcs(sprig.FuncMap()).Funcs(GenericFuncMap(ctx))
	t, err = t.Parse(strings.ReplaceAll(goTmpl.Template, "\n", ""))
	if err != nil {
		return nil, err
	}
	for _, tmpl := range t.Templates() {
		if tmpl.Tree == nil {
			continue
		}
		if cmd := findDictCommand(tmpl.Tree.Root); cmd != nil {
			return buildTemplateDict(cmd), nil
		}
	}
	return nil, nil
}

func findDictCommand(node parse.Node) *parse.CommandNode {
	switch n := node.(type) {
	case nil:
		return nil
	case *parse.ListNode:
		if n == nil {
			return nil
		}
		for _, child := range n.Nodes {
			if found := findDictCommand(child); found != nil {
				return found
			}
		}
	case *parse.ActionNode:
		return findDictCommand(n.Pipe)
	case *parse.PipeNode:
		if n == nil {
			return nil
		}
		for _, cmd := range n.Cmds {
			if found := findDictCommand(cmd); found != nil {
				return found
			}
		}
	case *parse.CommandNode:
		if len(n.Args) > 0 {
			if id, ok := n.Args[0].(*parse.IdentifierNode); ok && id.Ident == "dict" {
				return n
			}
		}
		for _, arg := range n.Args {
			if found := findDictCommand(arg); found != nil {
				return found
			}
		}
	case *parse.IfNode:
		return findBranchDictCommand(n.BranchNode)
	case *parse.RangeNode:
		return findBranchDictCommand(n.BranchNode)
	case *parse.WithNode:
		return findBranchDictCommand(n.BranchNode)
	case *parse.TemplateNode:
		return findDictCommand(n.Pipe)
	}
	return nil
}

func findBranchDictCommand(branch parse.BranchNode) *parse.CommandNode {
	if found := findDictCommand(branch.Pipe); found != nil {
		return found
	}
	if branch.List != nil {
		if found := findDictCommand(branch.List); found != nil {
			return found
		}
	}
	if branch.ElseList != nil {
		if found := findDictCommand(branch.ElseList); found != nil {
			return found
		}
	}
	return nil
}

func buildTemplateDict(cmd *parse.CommandNode) *TemplateDict {
	td := &TemplateDict{Children: make(map[string]*TemplateDict)}
	args := cmd.Args[1:]
	if len(args)%2 != 0 {
		td.Dynamic = true
	}
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(*parse.StringNode)
		if !ok {
			td.Dynamic = true
			continue
		}
		td.Keys = append(td.Keys, key.Text)
		if child := findDictCommand(args[i+1]); child != nil {
			td.Children[key.Text] = buildTemplateDict(child)
		}
	}
	return td
}

func (goTmpl *GoTemplate) Execute(ctx context.Context, obj interface{}, outputFormat string) (output interface{}, err error) {
	logs.WithContext(ctx).Debug("Execute - Start")
	buf := &bytes.Buffer{}
	goTmpl.Template = strings.ReplaceAll(goTmpl.Template, "\n", "")
	t := template.New(goTmpl.Name).Funcs(sprig.FuncMap()).Funcs(GenericFuncMap(ctx))
	t, err = t.Parse(goTmpl.Template)
	if err != nil {
		err = logs.Err(ctx, err, "")
		return "", err
	}
	if err = t.Execute(buf, obj); err != nil {
		err = logs.Err(ctx, err, "")
		return "", err
	}
	str := buf.String()
	switch outputFormat {
	case "string":
		if str == "<no value>" {
			logs.WithContext(ctx).Warn("template returned <no value>")
			return "", err
		}
		return str, nil
	case "json":
		if str == "<no value>" {
			return nil, err
		}
		if err = json.Unmarshal([]byte(str), &output); err != nil {
			err = fmt.Errorf("unable to marhsal templated output to JSON : %s, %s", buf.String(), err)
			logs.Err(ctx, err, "")
			return nil, err
		} else {
			return
		}
	}
	err = errors.New(fmt.Sprint("Unknown output format : ", outputFormat))
	logs.Err(ctx, err, "")
	return nil, err
}

func (goTmpl *GoTemplate) ExecuteWithErrors(ctx context.Context, obj interface{}, outputFormat string) (output interface{}, err error) {
	logs.WithContext(ctx).Debug("Execute - Start")
	buf := &bytes.Buffer{}

	t := template.New(goTmpl.Name).Funcs(sprig.FuncMap()).Funcs(GenericFuncMap(ctx))
	t, err = t.Parse(goTmpl.Template)
	if err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	if err = t.Execute(buf, obj); err != nil {
		logs.WithContext(ctx).Error(err.Error())
		return "", err
	}
	str := buf.String()
	switch outputFormat {
	case "string":
		if str == "<no value>" {
			logs.WithContext(ctx).Warn("template returned <no value>")
			return "", err
		}
		return str, nil
	case "json":
		if str == "<no value>" {
			return nil, err
		}
		if err = json.Unmarshal([]byte(str), &output); err != nil {
			err = fmt.Errorf("unable to marhsal templated output to JSON : %s, %s", buf.String(), err)
			logs.WithContext(ctx).Error(err.Error())
			return nil, err
		} else {
			return
		}
	}
	err = errors.New(fmt.Sprint("Unknown output format : ", outputFormat))
	logs.WithContext(ctx).Error(err.Error())
	return nil, err
}

func EvalFilter(ctx context.Context, filter map[string]interface{}, record map[string]interface{}) (result bool, err error) {
	for k, v := range filter {
		kk := fetchKey(k)
		if kk == "$or" || kk == "or" {
			if vArray, vArrayOk := v.([]interface{}); vArrayOk {
				result, err = evalOrFilter(ctx, vArray, record)
				if !result {
					return false, nil
				}
			} else {
				err = errors.New("$or needs an array")
				logs.Err(ctx, err, "")
				return false, err
			}
		} else {
			if recordValue, recordValueOk := record[kk]; recordValueOk {
				if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
					result, err = evalCondition(ctx, vMap, recordValue)
					if !result {
						return false, err
					}
				} else {
					result = eruutils.ImplCompare(recordValue, v)
					if !result {
						return false, err
					}
				}
			} else {
				err = errors.New(fmt.Sprint("key : ", kk, " not found in data"))
				logs.Err(ctx, err, "")
				return false, nil
			}
		}
	}
	return true, nil
}
func evalCondition(ctx context.Context, cond map[string]interface{}, recordValue interface{}) (result bool, err error) {
	for ck, cv := range cond {
		switch ck {
		case "$in", "$inc", "in", "inc":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				if rArray, rArrayOk := recordValue.([]interface{}); rArrayOk {
					return eruutils.ImplArrayContains(cvArray, rArray), nil
				} else {
					return eruutils.ImplContains(cvArray, recordValue), nil
				}
			} else {
				err = errors.New("$in and $inc operators requires an array")
				logs.Err(ctx, err, "")
				return false, err
			}
		case "$nin", "$ninc", "nin", "ninc":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				if rArray, rArrayOk := recordValue.([]interface{}); rArrayOk {
					return !(eruutils.ImplArrayContains(cvArray, rArray)), nil
				} else {
					return !(eruutils.ImplContains(cvArray, recordValue)), nil
				}
			} else {
				err = errors.New("$nin and $ninc operators requires an array")
				logs.Err(ctx, err, "")
				return false, err
			}
		case "$like", "like":
			if rvStr, rvStrOk := recordValue.(string); rvStrOk {
				if cvStr, cvStrOk := cv.(string); cvStrOk {
					return strings.Contains(rvStr, cvStr), nil
				} else {
					err = errors.New("$like operator requires a string")
					logs.Err(ctx, err, "")
					return false, err
				}
			} else {
				err = errors.New("$like operator requires a string to compare")
				logs.Err(ctx, err, "")
				return false, err
			}
		case "$nlike", "nlike":
			if rvStr, rvStrOk := recordValue.(string); rvStrOk {
				if cvStr, cvStrOk := cv.(string); cvStrOk {
					return !(strings.Contains(rvStr, cvStr)), nil
				} else {
					err = errors.New("$nlike operator requires a string")
					logs.Err(ctx, err, "")
					return false, err
				}
			} else {
				err = errors.New("$nlike operator requires a string to compare")
				logs.Err(ctx, err, "")
				return false, err
			}
		case "$gt", "gt":
			if rvF, rvFOk := recordValue.(float64); rvFOk {
				if cvF, cvFOk := cv.(float64); cvFOk {
					return (rvF > cvF), nil
				} else {
					err = errors.New("$gt operator requires a number")
					logs.Err(ctx, err, "")
					return false, nil
				}
			} else {
				err = errors.New("$gt operator requires a number to compare")
				logs.Err(ctx, err, "")
				return false, nil
			}
		case "$gte", "gte":
			if rvF, rvFOk := recordValue.(float64); rvFOk {
				if cvF, cvFOk := cv.(float64); cvFOk {
					return (rvF >= cvF), nil
				} else {
					err = errors.New("$gte operator requires a number")
					logs.Err(ctx, err, "")
					return false, nil
				}
			} else {
				err = errors.New("$gte operator requires a number to compare")
				logs.Err(ctx, err, "")
				return false, err
			}
		case "$lt", "lt":
			if rvF, rvFOk := recordValue.(float64); rvFOk {
				if cvF, cvFOk := cv.(float64); cvFOk {
					return (rvF < cvF), nil
				} else {
					err = errors.New("$lt operator requires a number")
					logs.Err(ctx, err, "")
					return false, nil
				}
			} else {
				err = errors.New("$lt operator requires a number to compare")
				logs.Err(ctx, err, "")
				return false, nil
			}
		case "$lte", "lte":
			if rvF, rvFOk := recordValue.(float64); rvFOk {
				if cvF, cvFOk := cv.(float64); cvFOk {
					return (rvF <= cvF), nil
				} else {
					err = errors.New("$lte operator requires a number")
					logs.Err(ctx, err, "")
					return false, nil
				}
			} else {
				err = errors.New("$lte operator requires a number to compare")
				logs.Err(ctx, err, "")
				return false, nil
			}
		case "$ne", "ne":
			return !(eruutils.ImplCompare(cv, recordValue)), nil
		case "$eq", "eq":
			return eruutils.ImplCompare(cv, recordValue), nil
		case "$jin", "jin":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				if rArray, rArrayOk := recordValue.([]interface{}); rArrayOk {
					return eruutils.ImplArrayContains(cvArray, rArray), nil
				} else {
					err = errors.New("$jin operator requires an array for record value")
					logs.Err(ctx, err, "")
					return false, err
				}
			} else {
				err = errors.New("$jin operator requires an array for filter")
				logs.Err(ctx, err, "")
				return false, err
			}
		case "$jnin", "jnin":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				if rArray, rArrayOk := recordValue.([]interface{}); rArrayOk {
					return eruutils.ImplArrayNotContains(cvArray, rArray), nil
				} else {
					err = errors.New("$jnin operator requires an array for record value")
					logs.Err(ctx, err, "")
					return false, err
				}
			} else {
				err = errors.New("$jnin operator requires an array for filter")
				logs.Err(ctx, err, "")
				return false, err
			}
		default:
			logs.Err(ctx, errors.New("operator not found"), "")
			return false, nil
		}
	}
	return
}
func fetchKey(k string) (key string) {
	key = k
	kArray := strings.Split(k, "___")
	if len(kArray) > 1 {
		key = kArray[1]
	}
	return
}
func evalOrFilter(ctx context.Context, filter []interface{}, record map[string]interface{}) (result bool, err error) {
	for _, v := range filter {
		if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
			result, err = EvalFilter(ctx, vMap, record)
		} else {
			err = errors.New("$or needs array of objects")
			logs.Err(ctx, err, "")
		}
		if result {
			return true, nil
		}
	}
	return
}

func fetchFilterKeys(ctx context.Context, inPutfilterStr string, parentPrefix string) (filterKeys []string, err error) {
	filter := make(map[string]interface{})
	err = json.Unmarshal([]byte(inPutfilterStr), &filter)
	if err != nil {
		logs.Err(ctx, err, "")
		return
	}
	return fetchFilterKeysFromMap(ctx, filter, parentPrefix)
}
func fetchFilterKeysFromMap(ctx context.Context, filter map[string]interface{}, parentPrefix string) (filterKeys []string, err error) {
	var tempStr []string
	for k, v := range filter {
		kk := fetchKey(k)
		if kk == "$or" {
			if vArray, vArrayOk := v.([]interface{}); vArrayOk {
				tempStr, err = fetchOrFilterKeys(ctx, vArray, parentPrefix)
				filterKeys = append(filterKeys, tempStr...)
			} else {
				err = errors.New("$or needs an array")
				logs.Err(ctx, err, "")
				return
			}
		} else {
			if parentPrefix == "" || strings.HasPrefix(kk, parentPrefix) {
				kk = strings.Replace(kk, parentPrefix, "", -1)
				filterKeys = append(filterKeys, kk)
			}
		}
	}
	return
}
func fetchOrFilterKeys(ctx context.Context, filter []interface{}, parentPrefix string) (orKeys []string, err error) {
	var tmpKeys []string
	for _, v := range filter {
		if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
			tmpKeys, err = fetchFilterKeysFromMap(ctx, vMap, parentPrefix)
			if len(tmpKeys) > 0 {
				orKeys = append(orKeys, tmpKeys...)
			}
		} else {
			err = errors.New("$or needs array of objects")
			logs.Err(ctx, err, "")
		}
	}
	return
}
func makeFilter(ctx context.Context, inPutfilterStr string, jsonKey string) (filterStr string, err error) {
	filter := make(map[string]interface{})
	err = json.Unmarshal([]byte(inPutfilterStr), &filter)
	if err != nil {
		logs.Err(ctx, err, "")
		return "false", nil
	}
	return makeFilterFromMap(ctx, filter, jsonKey, "")
}
func makeParentFilter(ctx context.Context, inPutfilterStr string, jsonKey string, parentPrefix string) (filterStr string, err error) {
	filter := make(map[string]interface{})
	err = json.Unmarshal([]byte(inPutfilterStr), &filter)
	if err != nil {
		logs.Err(ctx, err, "")
		return "false", nil
	}
	return makeFilterFromMap(ctx, filter, jsonKey, parentPrefix)
}

func makeFilterV2(ctx context.Context, inPutfilterStr string, jsonKey string, parentPrefix string, parentNamesStr string, defaultPrefix string) (filterStr string, err error) {
	filter := make(map[string]interface{})
	err = json.Unmarshal([]byte(inPutfilterStr), &filter)
	if err != nil {
		logs.Err(ctx, err, "")
		return "false", nil
	}
	var parentNames []map[string]interface{}
	if strings.TrimSpace(parentNamesStr) != "" {
		if err = json.Unmarshal([]byte(parentNamesStr), &parentNames); err != nil {
			logs.Err(ctx, err, "")
			return "false", nil
		}
	}
	return makeFilterFromMapV2(ctx, filter, jsonKey, parentPrefix, parentNames, defaultPrefix)
}

func matchParentPrefix(kk string, parentNames []map[string]interface{}, defaultPrefix string) (prefix string, fieldKey string) {
	prefix = defaultPrefix
	fieldKey = kk
	if idx := strings.Index(kk, "~"); idx >= 0 {
		before := kk[:idx]
		after := kk[idx+1:]
		for _, pn := range parentNames {
			for pk, pv := range pn {
				if strings.HasSuffix(pk, "_ef") {
					if pvStr, pvOk := pv.(string); pvOk && strings.TrimPrefix(pvStr, "_") == before {
						prefix = strings.TrimSuffix(pk, "_ef")
						fieldKey = after
					}
				}
			}
		}
	}
	return
}

func makeFilterFromMapV2(ctx context.Context, filter map[string]interface{}, jsonKey string, parentPrefix string, parentNames []map[string]interface{}, defaultPrefix string) (filterStr string, err error) {
	var filterStrArray []string
	tempStr := ""
	for k, v := range filter {
		kk := fetchKey(k)
		if kk == "$or" {
			if vArray, vArrayOk := v.([]interface{}); vArrayOk {
				tempStr, err = makeOrStringV2(ctx, vArray, jsonKey, parentPrefix, parentNames, defaultPrefix)
				filterStrArray = append(filterStrArray, tempStr)
			} else {
				err = errors.New("$or needs an array")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		} else {
			include := false
			isJson := false
			if parentPrefix == "" || strings.HasPrefix(kk, parentPrefix) {
				include = true
				kk = strings.Replace(kk, parentPrefix, "", -1)
			}
			if include {
				prefix, fieldKey := matchParentPrefix(kk, parentNames, defaultPrefix)
				kk = fieldKey
				if jsonKey != "" {
					colRef := jsonKey
					if prefix != "" {
						colRef = fmt.Sprint(prefix, ".", jsonKey)
					}
					kk = fmt.Sprint(colRef, "->>'", kk, "'")
					isJson = true
				} else if prefix != "" {
					kk = fmt.Sprint(prefix, ".", kk)
				}
				if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
					tempStr, err = makeFilterStr(ctx, kk, vMap, isJson)
					filterStrArray = append(filterStrArray, tempStr)
				} else if vF, vFOk := v.(float64); vFOk {
					if isJson {
						kk = fmt.Sprint("(", kk, ")::numeric")
					}
					tempStr = fmt.Sprint(kk, " = ", vF)
					filterStrArray = append(filterStrArray, tempStr)
				} else if vB, vBOk := v.(bool); vBOk {
					if isJson {
						kk = fmt.Sprint("(", kk, ")::bool")
					}
					tempStr = fmt.Sprint(kk, " = ", vB)
					filterStrArray = append(filterStrArray, tempStr)
				} else if vS, vSOk := v.(string); vSOk {
					if vBB, bErr := strconv.ParseBool(vS); bErr == nil {
						if isJson {
							kk = fmt.Sprint("(", kk, ")::bool")
						}
						tempStr = fmt.Sprint(kk, " = ", vBB)
						filterStrArray = append(filterStrArray, tempStr)
					} else {
						tempStr = fmt.Sprint(kk, " = '", v, "'")
						filterStrArray = append(filterStrArray, tempStr)
					}
				} else {
					tempStr = fmt.Sprint(kk, " = '", v, "'")
					filterStrArray = append(filterStrArray, tempStr)
				}
			}
		}
	}
	filterStr = strings.Join(filterStrArray, " and ")
	if filterStr != "" {
		filterStr = fmt.Sprint("(", filterStr, ")")
	}
	return filterStr, nil
}

func makeOrStringV2(ctx context.Context, filter []interface{}, jsonKey string, parentPrefix string, parentNames []map[string]interface{}, defaultPrefix string) (orStr string, err error) {
	orStr = ""
	orOp := ""
	filterStr := ""
	for i, v := range filter {
		if i > 0 {
			orOp = " or "
		}
		if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
			filterStr, err = makeFilterFromMapV2(ctx, vMap, jsonKey, parentPrefix, parentNames, defaultPrefix)
		} else {
			err = errors.New("$or needs array of objects")
			logs.Err(ctx, err, "")
		}
		if filterStr != "" {
			orStr = fmt.Sprint(orStr, orOp, filterStr)
		}
	}
	if orStr != "" {
		orStr = fmt.Sprint("(", orStr, ")")
	}
	return
}

func MakeFilterFromMap(ctx context.Context, filter map[string]interface{}, jsonKey string, parentPrefix string) (filterStr string, err error) {
	return makeFilterFromMap(ctx, filter, jsonKey, parentPrefix)
}

func makeFilterFromMap(ctx context.Context, filter map[string]interface{}, jsonKey string, parentPrefix string) (filterStr string, err error) {
	var filterStrArray []string
	tempStr := ""
	for k, v := range filter {
		kk := fetchKey(k)
		if kk == "$or" {
			if vArray, vArrayOk := v.([]interface{}); vArrayOk {
				tempStr, err = makeOrString(ctx, vArray, jsonKey, parentPrefix)
				filterStrArray = append(filterStrArray, tempStr)
			} else {
				err = errors.New("$or needs an array")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		} else {
			include := false
			isJson := false
			if parentPrefix == "" || strings.HasPrefix(kk, parentPrefix) {
				include = true
				kk = strings.Replace(kk, parentPrefix, "", -1)
			}
			if include {
				if jsonKey != "" {
					kk = fmt.Sprint(jsonKey, "->>'", kk, "'")
					isJson = true
				}
				if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
					tempStr, err = makeFilterStr(ctx, kk, vMap, isJson)
					filterStrArray = append(filterStrArray, tempStr)
				} else if vF, vFOk := v.(float64); vFOk {
					if isJson {
						kk = fmt.Sprint("(", kk, ")::numeric")
					}
					tempStr = fmt.Sprint(kk, " = ", vF)
					filterStrArray = append(filterStrArray, tempStr)
				} else if vB, vBOk := v.(bool); vBOk {
					if isJson {
						kk = fmt.Sprint("(", kk, ")::bool")
					}
					tempStr = fmt.Sprint(kk, " = ", vB)
					filterStrArray = append(filterStrArray, tempStr)
				} else if vS, vSOk := v.(string); vSOk {
					if vBB, bErr := strconv.ParseBool(vS); bErr == nil {
						if isJson {
							kk = fmt.Sprint("(", kk, ")::bool")
						}
						tempStr = fmt.Sprint(kk, " = ", vBB)
						filterStrArray = append(filterStrArray, tempStr)
					} else {
						tempStr = fmt.Sprint(kk, " = '", v, "'")
						filterStrArray = append(filterStrArray, tempStr)
					}
				} else {
					tempStr = fmt.Sprint(kk, " = '", v, "'")
					filterStrArray = append(filterStrArray, tempStr)
				}
			}
		}
	}
	filterStr = strings.Join(filterStrArray, " and ")
	if filterStr != "" {
		filterStr = fmt.Sprint("(", filterStr, ")")
	}
	return filterStr, nil
}

func makeOrString(ctx context.Context, filter []interface{}, jsonKey string, parentPrefix string) (orStr string, err error) {
	orStr = ""
	orOp := ""
	filterStr := ""
	for i, v := range filter {
		if i > 0 {
			orOp = " or "
		}
		if vMap, vMapOk := v.(map[string]interface{}); vMapOk {
			filterStr, err = makeFilterFromMap(ctx, vMap, jsonKey, parentPrefix)
		} else {
			err = errors.New("$or needs array of objects")
			logs.Err(ctx, err, "")
		}
		if filterStr != "" {
			orStr = fmt.Sprint(orStr, orOp, filterStr)
		}
	}
	if orStr != "" {
		orStr = fmt.Sprint("(", orStr, ")")
	}
	return
}

func makeFilterStr(ctx context.Context, key string, cond map[string]interface{}, isJson bool) (filterStr string, err error) {
	for ck, cv := range cond {
		switch ck {

		case "$btw":
			if btwMap, ok := cv.(map[string]interface{}); ok {
				from, fromOk := btwMap["from"]
				to, toOk := btwMap["to"]
				if fromOk && toOk {
					if cvF, cvFOk := from.(float64); cvFOk {
						if isJson {
							key = fmt.Sprint("(", key, ")::numeric")
						}
						filterStr = fmt.Sprint(key, " between ", cvF, " and ", to)
						return filterStr, nil
					} else {
						filterStr = fmt.Sprint(key, " between '", from, "' and '", to, "'")
						return filterStr, nil
					}
				} else {
					err = errors.New("'from' or 'to' key missing in $btw map")
					logs.Err(ctx, err, "")
					return "false", nil
				}
			} else {
				err = errors.New("$btw operator expects a map with 'from' and 'to' keys")
				logs.Err(ctx, err, "")
				return "false", nil

			}
		case "$in", "$inc":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				tempStr := ""
				tempSep := ""

				if len(cvArray) > 0 && isJson {
					if _, vFOk := cvArray[0].(float64); vFOk {
						key = fmt.Sprint("(", key, ")::numeric")
					} else if _, vBOk := cvArray[0].(bool); vBOk {
						key = fmt.Sprint("(", key, ")::bool")
					}
				}
				for i, v := range cvArray {
					if i > 0 {
						tempSep = ","
					}
					if vS, vSOk := v.(string); vSOk {
						tempStr = fmt.Sprint(tempStr, tempSep, "'", vS, "'")
					} else {
						tempStr = fmt.Sprint(tempStr, tempSep, vS)
					}
				}
				filterStr = fmt.Sprint(key, " in (", tempStr, ")")
				return filterStr, nil
			} else {
				err = errors.New("$in and $inc operators requires an array")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		case "$nin", "$ninc":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				tempStr := ""
				tempSep := ""
				if len(cvArray) > 0 && isJson {
					if _, vFOk := cvArray[0].(float64); vFOk {
						key = fmt.Sprint("(", key, ")::numeric")
					} else if _, vBOk := cvArray[0].(bool); vBOk {
						key = fmt.Sprint("(", key, ")::bool")
					}
				}
				for i, v := range cvArray {
					if i > 0 {
						tempSep = ","
					}
					if vS, vSOk := v.(string); vSOk {
						tempStr = fmt.Sprint(tempStr, tempSep, "'", vS, "'")
					} else {
						tempStr = fmt.Sprint(tempStr, tempSep, vS)
					}
				}
				filterStr = fmt.Sprint(key, " not in (", tempStr, ")")
				return filterStr, nil
			} else {
				err = errors.New("$nin and $ninc operators requires an array")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		case "$like":
			if cvStr, cvStrOk := cv.(string); cvStrOk {
				filterStr = fmt.Sprint(key, " like '%", cvStr, "%'")
				return filterStr, nil
			} else {
				err = errors.New("$like operator requires a string")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		case "$nlike":
			if cvStr, cvStrOk := cv.(string); cvStrOk {
				filterStr = fmt.Sprint(key, " not like '%", cvStr, "%'")
				return filterStr, nil
			} else {
				err = errors.New("$nlike operator requires a string")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		case "$gt":
			if cvF, cvFOk := cv.(float64); cvFOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::numeric")
				}
				filterStr = fmt.Sprint(key, " > ", cvF)
				return filterStr, nil
			} else {
				filterStr = fmt.Sprint(key, " > '", cv, "'")
				return filterStr, nil
			}
		case "$gte":
			if cvF, cvFOk := cv.(float64); cvFOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::numeric")
				}
				filterStr = fmt.Sprint(key, " >= ", cvF)
				return filterStr, nil
			} else {
				filterStr = fmt.Sprint(key, " >= '", cv, "'")
				return filterStr, nil
			}
		case "$lt":
			if cvF, cvFOk := cv.(float64); cvFOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::numeric")
				}
				filterStr = fmt.Sprint(key, " < ", cvF)
				return filterStr, nil
			} else {
				filterStr = fmt.Sprint(key, " < '", cv, "'")
				return filterStr, nil
			}
		case "$lte":
			if cvF, cvFOk := cv.(float64); cvFOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::numeric")
				}
				filterStr = fmt.Sprint(key, " <= ", cvF)
				return filterStr, nil
			} else {
				filterStr = fmt.Sprint(key, " <= '", cv, "'")
				return filterStr, nil
			}
		case "$ne":
			if _, cvFOk := cv.(float64); cvFOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::numeric")
				}
			} else if _, cvBOk := cv.(bool); cvBOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::bool")
				}
			}
			if cvS, cvSOk := cv.(string); cvSOk {
				filterStr = fmt.Sprint(key, " <> '", cvS, "'")
			} else {
				filterStr = fmt.Sprint(key, " <> ", cv)
			}
			return filterStr, nil
		case "$eq":
			if _, cvFOk := cv.(float64); cvFOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::numeric")
				}
			} else if _, cvBOk := cv.(bool); cvBOk {
				if isJson {
					key = fmt.Sprint("(", key, ")::bool")
				}
			}
			if cvS, cvSOk := cv.(string); cvSOk {
				filterStr = fmt.Sprint(key, " = '", cvS, "'")
			} else {
				filterStr = fmt.Sprint(key, " = ", cv)
			}
			return filterStr, nil
		case "$jin":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				tempStr := ""
				tempSep := ""

				for i, v := range cvArray {
					if i > 0 {
						tempSep = ","
					}
					if vS, vSOk := v.(string); vSOk {
						tempStr = fmt.Sprint(tempStr, tempSep, "'", vS, "'")
					} else {
						tempStr = fmt.Sprint(tempStr, tempSep, vS)
					}
				}
				key = strings.Replace(key, "->>", "->", -1)
				filterStr = fmt.Sprint(key, "  ?| array[", tempStr, "]")
				return filterStr, nil
			} else {
				err = errors.New("$jin operator requires an array")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		case "$jnin":
			if cvArray, cvArrayOk := cv.([]interface{}); cvArrayOk {
				tempStr := ""
				tempSep := ""

				for i, v := range cvArray {
					if i > 0 {
						tempSep = ","
					}
					if vS, vSOk := v.(string); vSOk {
						tempStr = fmt.Sprint(tempStr, tempSep, "'", vS, "'")
					} else {
						tempStr = fmt.Sprint(tempStr, tempSep, vS)
					}
				}
				key = strings.Replace(key, "->>", "->", -1)
				filterStr = fmt.Sprint(" not (", key, " ?| array[", tempStr, "])")
				return filterStr, nil
			} else {
				err = errors.New("$jnin operator requires an array")
				logs.Err(ctx, err, "")
				return "false", nil
			}
		case "$null":
			v := "is null"
			var b bool
			if cvB, cvBOk := cv.(bool); cvBOk {
				b = cvB
			} else if cvS, cvSOk := cv.(string); cvSOk {
				if cvSB, bErr := strconv.ParseBool(cvS); bErr == nil {
					b = cvSB
				} else {
					return "false", nil
				}
			} else {
				return "false", nil
			}
			if !b {
				v = "is not null"
			}
			if isJson {
				key = fmt.Sprint("(", key, ") ", v)
			} else {
				key = fmt.Sprint(key, " ", v)
			}
			filterStr = key
			return filterStr, nil
		default:
			logs.Err(ctx, errors.New("operator not found"), "")
			return "false", nil
		}
	}
	return
}

func excelToJson(ctx context.Context, fData string, sheetNames string, firstRowHeader string, headers string, mapKeys string) (fJson interface{}, err error) {
	sheetNameArray := strings.Split(sheetNames, ",")
	sheetHeadersArray := strings.Split(headers, ",")
	firstRowHeaderArray := strings.Split(firstRowHeader, ",")
	mapKeysArray := strings.Split(mapKeys, ",")

	result := make(map[string][]map[string]interface{})
	fDataDecoded, err := b64.StdEncoding.DecodeString(fData)
	if err != nil {
		logs.Err(ctx, err, "")
		return "", nil
	}
	f, err := excelize.OpenReader(bytes.NewReader(fDataDecoded))
	if err != nil {
		logs.Err(ctx, err, "")
		return "", err
	}
	defer func() {
		// Close the spreadsheet.
		if err := f.Close(); err != nil {
			logs.Err(ctx, err, "")
		}
	}()
	sheetFound := false
	if sheetNameArray[0] == "" {
		sheetFound = true
	}
	for sNo, sheetName := range f.GetSheetList() {
		outputSheetName := sheetName
		for _, sn := range sheetNameArray {
			if sn == sheetName {
				sheetFound = true
				break
			}
		}
		if sheetFound {
			if sNo < len(mapKeysArray) {
				if mapKeysArray[sNo] != "" {
					outputSheetName = mapKeysArray[sNo]
				}
			}
			resultRow := make(map[string]interface{})
			isFirstRowHeader := false
			if sNo < len(firstRowHeaderArray) {
				isFirstRowHeader, err = strconv.ParseBool(firstRowHeaderArray[sNo])
				if err != nil {
					logs.Err(ctx, err, "")
					isFirstRowHeader = false
				}
			}
			if sNo >= len(sheetHeadersArray) && !isFirstRowHeader {
				resultRow["error"] = "header information missing"
				result[outputSheetName] = append(result[outputSheetName], resultRow)
			} else if sheetHeadersArray[0] == "" && !isFirstRowHeader {
				resultRow["error"] = "header information missing"
				result[outputSheetName] = append(result[outputSheetName], resultRow)
			} else {
				var keys []string
				if sNo < len(sheetHeadersArray) {
					keys = strings.Split(sheetHeadersArray[sNo], "|")
				}

				rows, rErr := f.GetRows(sheetName)
				if rErr != nil {
					err = rErr
					logs.Err(ctx, err, "")
					return
				}
				for rNo, row := range rows {
					resultRow = make(map[string]interface{})
					skipRow := false
					for cNo, colCell := range row {
						skipRow = false
						if rNo == 0 && isFirstRowHeader {
							skipRow = true
							if cNo >= len(keys) {
								keys = append(keys, colCell)
							} else if keys[cNo] == "" {
								keys[cNo] = colCell
							}
						} else {
							k := ""
							if cNo >= len(keys) {
								k = fmt.Sprint("C", cNo)
							} else if keys[cNo] == "" {
								k = fmt.Sprint("C", cNo)
							} else {
								k = keys[cNo]
							}
							resultRow[k] = colCell
						}
					}
					if !skipRow {
						result[outputSheetName] = append(result[outputSheetName], resultRow)
					}
				}
			}
		}
	}
	return result, nil
}
