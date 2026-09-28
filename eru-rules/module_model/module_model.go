package module_model

type ModuleProjectI interface {
}

type Project struct {
	ProjectId string `eru:"required"`
	DMNs      map[string]DMN
	DataTypes map[string]DataType
}

type DMN struct {
	DmnName string `eru:"required"`
}

type DataType struct {
	Name         string `eru:"required"`
	Type         string `eru:"required"`
	IsArray      bool
	Constraint   string
	SubDataTypes map[string]DataType
}
