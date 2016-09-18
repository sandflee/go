package main

import (
	"fmt"
	"encoding/json"
)

type JsonType struct {
	AString string `json:"aaa"`
	BBool bool
	CMap map[string]string
	DArray []string
}

func main() {

	jType := &JsonType{AString:"string", BBool:false, CMap:map[string]string{"a":"b"}, DArray:[]string{"array1","array2"}}

	fmt.Println(jType)
	var a JsonType
	a.AString = "a"
	a.BBool = true
	a.CMap = make(map[string]string)
	a.CMap["hello"] = "a"
	a.DArray = make([]string, 0)
	a.DArray = append(a.DArray, "array")
	fmt.Println(a)
	if m, err := json.Marshal(jType); err == nil {
		fmt.Println(string(m))
	} else {
		fmt.Println("error,",err)
	}

}
