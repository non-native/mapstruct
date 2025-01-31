package mapstruct

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
)

const DefaultTag = "map"

func I2StrcutIfOk(vals interface{}, dst interface{}) error {
	mapData, ok := vals.(map[string]interface{})

	if !ok {
		return errors.New("type is not map[string]interface{}")
	}

	return Map2Struct(mapData, dst)
}

func Map2Struct(vals map[string]interface{}, dst interface{}) (err error) {
	return Map2StructTag(vals, dst, DefaultTag)
}

func Map2StructTag(vals map[string]interface{}, dst interface{}, tagName string) (err error) {
	defer func() {
		e := recover()
		if e != nil {
			if v, ok := e.(error); ok {
				err = errors.New("Panic: " + v.Error())
			} else {
				err = errors.New("Panic: " + reflect.ValueOf(e).String())
			}
		}
	}()

	pt := reflect.TypeOf(dst)
	pv := reflect.ValueOf(dst)

	if pv.Kind() != reflect.Ptr || pv.Elem().Kind() != reflect.Struct {
		return errors.New("Not a pointer of struct")
		//return fmt.Errorf("not a pointer of struct")
	}

	var f reflect.StructField
	var fv reflect.Value

	for i := 0; i < pt.Elem().NumField(); i++ {
		f = pt.Elem().Field(i)
		fv = pv.Elem().Field(i)

		if f.Anonymous {
			continue
		}

		if !fv.CanSet() {
			continue
		}

		tag := f.Tag.Get(tagName)
		name, option := parseTag(tag)

		if name == "_" {
			continue
		}

		if name == "" {
			// tag name is not set, use field name
			name = f.Name
		}

		err = map2Field(vals, fv, name, option)
		if err != nil {
			return errors.New("field " + name + "(" + fv.Type().Kind().String() + ") error: " + err.Error())
		}

		continue
	}

	return nil
}

func Map2Field(vals map[string]interface{}, dst interface{}, tag string) error {
	fv := reflect.ValueOf(dst)
	if fv.Kind() != reflect.Ptr {
		return errors.New("dst must be a pointer")
	}

	name, option := parseTag(tag)

	return map2Field(vals, fv, name, option)
}

func map2Field(vals map[string]interface{}, fv reflect.Value, name, option string) error {
	if name == "-" || name == "" {
		return nil // ignore "-"
	}

	// value from map
	val, ok := vals[name]
	if !ok {
		val, ok = vals[strings.ToLower(name)]
	}

	if !ok { // value not found
		if option == "required" {
			return errors.New("'" + name + "' is required")
		}

		if len(option) != 0 {
			val = option // 'option' means 'default value' here
		} else {
			return nil // ignore it
		}
	}

	return convert(val, fv)
}

// Convert varies types of value to a certain type.
// Value can be type of string or json or whatever type which is convertable to the target type.
func Convert(dst interface{}, val interface{}) error {
	fv := reflect.ValueOf(dst)
	if fv.Type().Kind() != reflect.Ptr {
		return errors.New("dst must be a pointer")
	}
	return convert(val, fv)
}

func convert(val interface{}, fv reflect.Value) (err error) {
	// assign or convert value to field
	if assignToField(val, fv) == nil {
		return nil
	}

	switch v := val.(type) {
	case string:
		// parse string to value
		s := strings.TrimSpace(v)
		err = convertStringToValue(s, fv, fv.Type().Kind())

	case json.RawMessage:
		// unmarshal json
		err = convertJsonToValue(v, fv)

	default:
		err = errors.New("value type is not supported: value=" + reflect.ValueOf(val).Kind().String())
	}

	return err
}

func assignToField(val interface{}, fv reflect.Value) error {
	vv := reflect.ValueOf(val)
	vt := reflect.TypeOf(val)
	ft := fv.Type()

	// assign or convert value to field
	if vt.AssignableTo(ft) {
		fv.Set(vv)
		return nil
	}
	if vt.ConvertibleTo(ft) {
		fv.Set(vv.Convert(ft))
		return nil
	}
	return errors.New("Can not assign: valueKind = " + vt.Kind().String())
	// Loss of info, below. Is it required or commonly useful? Worth fmt?
	//return fmt.Errorf("can not assign: value=%v(%v)", val, vt.Kind())
}

func convertJsonToValue(data json.RawMessage, fv reflect.Value) error {
	var err error

	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			fv.Set(reflect.New(fv.Type().Elem()))
		}
	} else {
		fv = fv.Addr()
	}

	err = json.Unmarshal(data, fv.Interface())

	if err != nil {
		return errors.New("Invalid json: " + err.Error() + ", " + string(data))
	}

	return nil
}

func convertStringToValue(s string, fv reflect.Value, kind reflect.Kind) error {
	if !fv.CanAddr() {
		return errors.New("Target can not addr")
	}

	if assignToField(s, fv) == nil {
		return nil
	}

	if kind == reflect.String {
		fv.SetString(s)
		return nil
	}

	if kind == reflect.Slice {
		return convertStringToSlice(s, fv)
	}

	if kind == reflect.Ptr || kind == reflect.Struct {
		return convertJsonToValue(json.RawMessage(s), fv)
	}

	if kind == reflect.Bool {
		switch strings.ToLower(s) {
		case "true":
			fv.SetBool(true)
		case "false":
			fv.SetBool(false)
		case "1":
			fv.SetBool(true)
		case "0":
			fv.SetBool(false)
		default:
			return errors.New("Invalid bool: value=" + s)
		}
		return nil
	}

	if reflect.Int <= kind && kind <= reflect.Int64 {
		i, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return errors.New("Invalid int: value=" + s)
		}
		fv.SetInt(i)

	} else if reflect.Uint <= kind && kind <= reflect.Uint64 {
		i, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return errors.New("Invalid uint: value=" + s)
		}
		fv.SetUint(i)

	} else if reflect.Float32 == kind || kind == reflect.Float64 {
		i, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return errors.New("Invalid float: value=" + s)
		}
		fv.SetFloat(i)

	} else {
		// type not support
		return errors.New("Type not supported: value=" + s)
	}
	return nil
}

func convertStringToSlice(s string, fv reflect.Value) error {
	var err error
	ft := fv.Type()
	et := ft.Elem()

	if len(s) == 0 {
		return nil
	}

	data := json.RawMessage(s)
	if data[0] == '[' && data[len(data)-1] == ']' {
		return convertJsonToValue(data, fv)
	}

	ss := strings.Split(s, ",")
	fs := reflect.MakeSlice(ft, 0, len(ss))

	for _, si := range ss {
		ev := reflect.New(et).Elem()

		err = convertStringToValue(si, ev, et.Kind())
		if err != nil {
			return err
		}
		fs = reflect.Append(fs, ev)
	}

	fv.Set(fs)

	return nil
}

func parseTag(tag string) (string, string) {
	tags := strings.Split(tag, ",")

	if len(tags) <= 0 {
		return "", ""
	}

	if len(tags) == 1 {
		return tags[0], ""
	}

	return tags[0], tags[1]
}
