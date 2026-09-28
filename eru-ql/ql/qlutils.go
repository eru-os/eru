package ql

func GetQL(queryType string) QL {
	switch queryType {
	case "graphql":
		return new(GraphQLData)
	case "sql":
		return new(SQLData)
	default:
		return nil
		//do nothing
	}
}

type OrderedMap struct {
	Rank     int
	Level    int
	SubLevel int
	Obj      map[string]interface{}
}

type MapSorter []*OrderedMap

func (a MapSorter) Len() int {
	return len(a)
}
func (a MapSorter) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}
func (a MapSorter) Less(i, j int) bool {
	return a[i].Level < a[j].Level
}
