package agents

const TemplateVarsSchemaString = `{"type":"object","properties":{"Headers":{"type":"object"},"FormData":{"type":"object"},"FileData":{"type":"object"},"Params":{"type":"object"},"Vars":{"type":"object","properties":{"Body":{"type":"object"},"OrgBody":{"type":"object"}},"required":[]},"Body":{"type":"object"},"OrgBody":{"type":"object"},"Token":{"type":"object"},"FormDataKeyArray":{"type":"array","items":[{"type":"string"}]},"LoopVars":{"type":"array","items":[{"type":"object"}]},"LoopVar":{"type":"object"},"Cookies":{"type":"object"},"ResponseStatus":{"type":"integer"}},"required":[]}`

const GoTemplateContextVariablePrompt = `

There are three attributes in the context variable :
1. Vars or vars : this is JSON object for the current function step and its type is of TemplateVars
2. ReqVars or req_vars : this is map of string as key and JSON object of type TemplateVars as value. The map key is the name of previous function steps. This holds all the previous REQUEST objects of previous function steps
3. ResVars or res_vars : this is map of string as key and JSON object of type TemplateVars as value. The map key is the name of previous function steps. This holds all the previous RESPONSE objects of previous function steps
TemplateVars JSON schema is as follows :

There are many custom functions written by us that we can use in the gotemplate.

Signatures are written as: name arg(type) arg(type) -> return(type). Arguments are
POSITIONAL - call them as {{funcName arg1 arg2}}, or pipe the last argument in
({{arg | funcName}}) only when the function takes a single argument. A type mismatch
fails the whole template, so convert first with stringToByte / bytesToString /
marshalJSON / stringify.

JSON Functions:
1. stringify j(any) -> string : marshal any value to its JSON string. {{stringify .Vars.Body}}
2. unquote s(string) -> string : strips one layer of surrounding quotes and unescapes.
3. marshalJSON j(any) -> []byte : marshal to JSON BYTES - pipe through bytesToString for a string.
4. unmarshalJSON b([]byte) -> any : parse JSON bytes into a map or array. {{unmarshalJSON (stringToByte .Vars.Body.payload)}}

Encoding Functions:
5. b64Encode str([]byte) -> string : standard base64 encode. Input must be BYTES.
6. b64Decode str(string) -> string : standard base64 decode; returns "" with NO error when the input is not valid base64.
7. hexEncode str([]byte) -> string : input must be BYTES.
8. hexDecode str(string) -> string

Crypto Functions:
9. aesEncryptGCM pb([]byte) k([]byte) -> []byte
10. aesDecryptGCM eb([]byte) k([]byte) -> []byte
11. aesEncryptECB pb([]byte) k([]byte) -> []byte
12. aesDecryptECB eb([]byte) k([]byte) -> []byte
13. aesEncryptCBC pb([]byte) k([]byte) iv([]byte) -> []byte
14. aesDecryptCBC eb([]byte) k([]byte) iv([]byte) -> []byte
15. generateAesKey bits(int) -> []byte : bits is 128, 192 or 256.
16. generate_rsa_keypair bits(int) -> object : returns an object holding the generated key pair.
17. encryptRSACert j([]byte) pubK(string) -> []byte : pubK is a PEM certificate.
18. hmac b(string) secret(string) -> []byte : returns BYTES - pipe through hexEncode or b64Encode.
19. shaHash b(string) bits(int) -> string : hex digest; bits MUST be 256 or 512, anything else errors.
20. md5 str(string) output(string) -> string : output MUST be "hex" or "string", anything else errors.
21. PKCS7Pad buf([]byte) size(int) -> []byte : size is the block size, e.g. 16.
22. PKCS7Unpad buf([]byte) -> []byte
23. new_jwt privateKeyStr(string) claimsMap(map) -> string : signs the claims with a PEM private key.

String/Data Functions:
24. bytesToString b([]byte) -> string
25. stringToByte s(string) -> []byte
26. doubleQuote s(string) -> string : wraps in quotes and quotes again, so the result embeds inside a JSON string.
27. len j(any) -> int : NOT the Go builtin - it marshals the value to JSON and returns the length of that JSON text. Use arrayLen to count array elements.
28. str_concat sep(string) inStr...(string) -> string : SEPARATOR FIRST, then any number of strings. {{str_concat "-" .Vars.Body.a .Vars.Body.b}}
29. str_replace txt(string) oldStr(string) newStr(string) num(int) -> string : num is the maximum number of replacements; -1 replaces all.
30. removenull txt(string) -> string : strips NUL (zero byte) characters, literal and escaped.
31. char_index s(string) c(string) -> int : index of c in s, -1 when absent.

Map/Array Functions:
32. saveVar vars(map) keyToSave(string) valueToSave(any) -> error : writes into the map in place and renders nothing.
33. concatMapKeyVal vars(map) keys([]string) seprator(string) -> string : joins the listed keys as key=value| - the seprator argument is currently ignored.
34. concatMapKeyValUnordered vars(map) seprator(string) keyFirst(bool) varSeprator(string) -> string : walks every key in map order; keyFirst=false emits value<seprator>key.
35. makeMapKeyValUnordered str(string) seprator(string) -> map : inverse of the above - splits on seprator, then each pair on "=".
36. overwriteMap orgMap(map) b([]byte) -> []byte : merges the JSON bytes over orgMap and returns the merged JSON BYTES.
37. removeMapKey orgMap(map) key(string) -> map
38. getMapValue orgMap(map) key(string) -> any : returns the whole map when the key is absent.
39. getMapKeys orgMap(map) -> []string : unordered.
40. getMapPointerValue orgMap(map of pointers) key(string) -> any
41. getArrayValue orgArray([]any) index(int) emptyValue(string) -> any : index is 0 based; emptyValue is "object", "string" or "number" and decides what is returned when the index is out of range.
42. arrayLen arr(any) -> int : element count; errors unless arr is an array. Use this, never len, to count items. {{arrayLen .ResVars.fetch_user.Body.rows}}
43. is_array arr(any) -> bool
44. sortMapArray mapArray([]any) sortKey(string) -> []any : ascending; every element must be a map and sortKey must hold a NUMBER.

Math Functions:
45. math_add args...(number) -> float : any number of numeric arguments.
46. math_sub a(number) b(number) -> float : a - b
47. math_div a(number) b(number) -> float : a / b
48. math_mul a(float) b(float) -> float : both arguments must already be floats.
49. math_round a(number) r(float) -> float : r is the rounding factor - 1 for whole numbers, 10 for one decimal, 100 for two.

Date/Time Functions:
50. current_date -> string : today as YYYY-MM-DD. Takes NO arguments.
51. date_diff indtstr(string) n(int) t(string) -> string : adds n to the date, negative subtracts; input and output are YYYY-MM-DD; t is "days", "months" or "years". {{date_diff .Vars.Body.dt -1 "months"}}
52. date_part dtStr(string) dtPart(string) -> string : dtStr is YYYY-MM-DD; dtPart is "DAY" (01-31), "MONTHN" (01-12), "MONTH" (name) or "YEAR".
53. date_format dtStr(string) srcLayout(string) newLayout(string) -> string : layouts are Go reference layouts, e.g. "2006-01-02" to "02/01/2006".

Utility Functions:
54. uuid -> string : new UUID. Takes NO arguments.
55. null -> any : JSON null. Takes NO arguments.
56. inc n(int) -> int : n + 1
57. logobject v(any) : logs the value as JSON and renders nothing.
58. logstring str(any) : info log, renders nothing.
59. logerror str(any) : error log, renders nothing.

Template/Filter Functions:
60. evalFilter filter(map) record(map) -> bool : true when the record satisfies the filter.
61. makeFilter filter(string) jsonKey(string) -> string : builds a SQL where clause from a JSON filter string; jsonKey is the JSON column the keys sit under.
62. makeParentFilter filter(string) jsonKey(string) parentPrefix(string) -> string : same, for keys nested under parentPrefix.
63. fetch_filter_keys filter(string) parentPrefix(string) -> []string : the keys referenced by a filter string.
64. execTemplate obj(any) templateString(string) outputFormat(string) -> any : runs a nested template against obj; outputFormat is "string" or "json".

File/Data Processing:
65. excelToJson fData(string) sheetNames(string) firstRowHeader(string) headers(string) keys(string) -> any : fData is the base64 encoded workbook; sheetNames, firstRowHeader, headers and keys are COMMA SEPARATED lists, one entry per sheet ("" means all sheets / defaults).
66. jsonToCsv mapObjs([]any) hasHeader(bool) -> string : array of maps to CSV text.
67. jsonToCsvB64 mapObjs([]any) hasHeader(bool) -> string : same as jsonToCsv, base64 encoded.
68. kmsDecrypt eStr([]byte) kmsStoreType(string) region(string) kmsId(string) kmsAlias(string) -> string
69. getObjDiff a(map) b(map) -> map : returns {"o": <values in a>, "n": <values in b>} for the keys present in both and differing.

SPRIG FUNCTIONS (Masterminds sprig v3.2.3)

Every function below is ALSO registered and callable, in addition to the 69 custom
functions above. There are no name clashes between the two sets. Semantics are the
standard sprig ones (https://masterminds.github.io/sprig/) - this is the exact list of
registered names, so treat it as closed: if a name is NOT on this list and NOT in the
custom list above, it does NOT exist and the template will fail to parse. Never invent
one, and never assume a Helm-only helper (include, tpl, toYaml, required, lookup) is
available - those are Helm additions, not sprig, and are NOT registered here.

Strings: abbrev abbrevboth camelcase cat contains hasPrefix hasSuffix indent initials
  kebabcase lower nindent nospace plural quote randAlpha randAlphaNum randAscii randBytes
  randNumeric repeat replace shuffle snakecase squote substr swapcase title trim trimAll
  trimPrefix trimSuffix trimall trunc untitle upper wrap wrapWith
String lists: join sortAlpha split splitList splitn toStrings
Math: add add1 addf add1f sub subf div divf mod mul mulf max maxf min minf biggest floor
  ceil round seq until untilStep toDecimal
Date: ago date dateInZone dateModify date_in_zone date_modify duration durationRound
  htmlDate htmlDateInZone must_date_modify mustDateModify toDate mustToDate now unixEpoch
Defaults and flow control: coalesce default empty fail ternary all any
Encoding: b32dec b32enc b64dec b64enc
Lists: list tuple append prepend push concat chunk compact first last initial rest
  reverse slice uniq has without mustAppend mustPrepend mustPush mustChunk mustCompact
  mustFirst mustLast mustInitial mustRest mustReverse mustSlice mustUniq mustHas
  mustWithout
Dictionaries: dict get set unset dig keys values hasKey pick omit pluck merge
  mergeOverwrite deepCopy mustMerge mustMergeOverwrite mustDeepCopy
Type conversion: atoi int int64 float64 toString toJson toPrettyJson toRawJson fromJson
  mustToJson mustToPrettyJson mustToRawJson mustFromJson
Paths: base dir ext clean isAbs osBase osDir osExt osClean osIsAbs
Regex: regexFind regexFindAll regexMatch regexSplit regexReplaceAll
  regexReplaceAllLiteral regexQuoteMeta mustRegexFind mustRegexFindAll mustRegexMatch
  mustRegexSplit mustRegexReplaceAll mustRegexReplaceAllLiteral
Crypto and certificates: sha1sum sha256sum adler32sum bcrypt htpasswd derivePassword
  encryptAES decryptAES genPrivateKey genCA genCAWithKey genSelfSignedCert
  genSelfSignedCertWithKey genSignedCert genSignedCertWithKey buildCustomCert
Reflection: kindIs kindOf typeIs typeIsLike typeOf deepEqual
Semver: semver semverCompare
UUID and misc: uuidv4 randInt urlJoin urlParse getHostByName hello
Environment: env expandenv

Choosing between the two sets:
  - Prefer a CUSTOM function when both could work - it is the one the platform is built
    around and it handles the Eru data shapes. In particular use stringify / marshalJSON
    rather than toJson, arrayLen rather than len, and the date_* functions rather than
    sprig date arithmetic, since inputs and outputs are YYYY-MM-DD.
  - Use sprig for what the custom set does not cover: string shaping, regex, list and
    dictionary manipulation, defaults and ternary, and type conversion.
  - "must" prefixed sprig functions return an error instead of silently swallowing it,
    which fails the step - prefer them when a silent wrong value would be worse.
  - Do NOT use env, expandenv, getHostByName or hello: they read the server environment
    or do nothing, and have no place in a function template.
`
