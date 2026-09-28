package common_types

type TableColsMetaData struct {
	TblSchema         string        `json:"tbl_schema" eru:"required"`
	TblName           string        `json:"tbl_name" eru:"required"`
	ColName           string        `json:"col_name" eru:"required"`
	DataType          string        `json:"data_type"`
	OwnDataType       string        `json:"own_data_type" eru:"required"`
	PrimaryKey        bool          `json:"primary_key" eru:"required"`
	IsUnique          bool          `json:"is_unique" eru:"required"`
	PkConstraintName  string        `json:"pk_constraint_name"`
	UqConstraintName  string        `json:"uq_constraint_name"`
	IsNullable        bool          `json:"is_nullable" eru:"required"`
	ColPosition       int           `json:"col_position"`
	DefaultValue      string        `json:"default_value"`
	AutoIncrement     bool          `json:"auto_increment"`
	CharMaxLength     int           `json:"char_max_length"`
	NumericPrecision  string        `json:"numeric_precision"`
	NumericScale      int           `json:"numeric_scale"`
	DatetimePrecision int           `json:"datetime_precision"`
	FkConstraintName  string        `json:"fk_constraint_name"`
	FkDeleteRule      string        `json:"fk_delete_rule"`
	FkTblSchema       string        `json:"fk_tbl_schema"`
	FkTblName         string        `json:"fk_tbl_name"`
	FkColName         string        `json:"fk_col_name"`
	ColumnMasking     ColumnMasking `json:"column_masking"`
}

type ColumnMasking struct {
	MaskingType string `json:"masking_type"`
}

// TableChangeType represents the type of change made to a table structure
type TableChangeType string

const (
	ChangeTypeAddColumn    TableChangeType = "ADD_COLUMN"
	ChangeTypeDropColumn   TableChangeType = "DROP_COLUMN"
	ChangeTypeModifyColumn TableChangeType = "MODIFY_COLUMN"
)

// ColumnChange represents a specific change to a column
type ColumnChange struct {
	ChangeType    TableChangeType    `json:"change_type"`
	ColumnName    string             `json:"column_name"`
	OldColumn     *TableColsMetaData `json:"old_column,omitempty"`
	NewColumn     *TableColsMetaData `json:"new_column,omitempty"`
	ChangedFields []string           `json:"changed_fields"`
}

// TableStructureDiff represents the differences between old and new table structures
type TableStructure struct {
	NewColumns      map[string]TableColsMetaData `json:"new_columns"`
	DroppedColumns  []string                     `json:"dropped_columns"`
	ModifiedColumns map[string]ColumnChange      `json:"modified_columns"`
}
