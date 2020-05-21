package utils

import (
	"reflect"
)

/**
将from的值复制到to
如果from不为空，则将值复制到to
*/
func CopyValue(to interface{}, from interface{}, emp func(interface{}) bool) {
	toElem := reflect.ValueOf(to).Elem()

	fromElem := reflect.ValueOf(from).Elem()
	fromType := fromElem.Type()

	for i := 0; i < fromElem.NumField(); i++ {
		formField := fromElem.Field(i)
		fromValue := formField.Interface()
		if !emp(fromValue) {
			// fmt.Printf("%d: %s %s = %v\n", i, fromType.Field(i).FileName, formField.Type(), fromValue)
			toElem.FieldByName(fromType.Field(i).Name).Set(formField)
		}
	}
}

/**
将from的值复制到to
如果to的值为空，则从from读取值
*/
func CopyValue2(to interface{}, from interface{}, emp func(interface{}) bool) {
	toElem := reflect.ValueOf(to).Elem()
	toType := toElem.Type()

	fromElem := reflect.ValueOf(from).Elem()

	for i := 0; i < toElem.NumField(); i++ {
		toField := toElem.Field(i)
		filedName := toType.Field(i).Name

		if toField.CanInterface() {
			toValue := toField.Interface()
			if emp(toValue) {
				fromFiled := fromElem.FieldByName(filedName)
				toField.Set(fromFiled)
			}
		}
	}
}

/**
判断值是否为空
*/
func EmpValue(value interface{}) bool {
	var arr = []interface{}{"", nil, 0, uint64(0), int64(0)}
	for _, a := range arr {
		if a == value {
			return true
		}
	}
	return false
}
