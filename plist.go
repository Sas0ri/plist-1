// Copyright 2012 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package plist implements parsing of Apple plist files.
package plist

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"reflect"
	"strconv"
	"time"
)

func next(data []byte) (skip, tag, rest []byte) {
	i := bytes.IndexByte(data, '<')
	if i < 0 {
		return data, nil, nil
	}
	j := bytes.IndexByte(data[i:], '>')
	if j < 0 {
		return data, nil, nil
	}
	j += i + 1
	return data[:i], data[i:j], data[j:]
}

func Unmarshal(data []byte, v interface{}) error {
	var tag []byte
	for {
		_, tag, data = next(data)
		if bytes.HasPrefix(tag, []byte("<?xml")) || bytes.HasPrefix(tag, []byte("<!DOCTYPE")) {
			// skip over declarations
			continue
		}
		if !bytes.HasPrefix(tag, []byte("<plist")) {
			return fmt.Errorf("not a plist")
		}
		break
	}

	data, err := unmarshalValue(data, reflect.ValueOf(v))
	if err != nil {
		return err
	}
	_, tag, data = next(data)
	if !bytes.Equal(tag, []byte("</plist>")) {
		return fmt.Errorf("junk on end of plist")
	}
	return nil
}

func unmarshalValue(data []byte, v reflect.Value) (rest []byte, err error) {
	_, tag, data := next(data)
	if tag == nil {
		return nil, fmt.Errorf("unexpected end of data")
	}

	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}

	switch string(tag) {
	case "<dict>":
		t := v.Type()
		if v.Kind() != reflect.Struct {
			return nil, fmt.Errorf("cannot unmarshal <dict> into non-struct %s", v.Type())
		}
	Dict:
		for {
			_, tag, data = next(data)
			if len(tag) == 0 {
				return nil, fmt.Errorf("eof inside <dict>")
			}
			if string(tag) == "</dict>" {
				break
			}
			if string(tag) != "<key>" {
				return nil, fmt.Errorf("unexpected tag %s inside <dict>", tag)
			}
			var body []byte
			body, tag, data = next(data)
			if len(tag) == 0 {
				return nil, fmt.Errorf("eof inside <dict>")
			}
			if string(tag) != "</key>" {
				return nil, fmt.Errorf("unexpected tag %s inside <dict>", tag)
			}
			name := string(body)
			var i int
			for i = 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.Name == name || f.Tag.Get("plist") == name {
					data, err = unmarshalValue(data, v.Field(i))
					if err != nil {
						return nil, err
					}
					continue Dict
				}
			}
			data, err = skipValue(data)
			if err != nil {
				return nil, err
			}
		}
		return data, nil

	case "<array>":
		t := v.Type()
		if v.Kind() != reflect.Slice {
			return nil, fmt.Errorf("cannot unmarshal <array> into non-slice %s", v.Type())
		}
		for {
			_, tag, rest := next(data)
			if len(tag) == 0 {
				return nil, fmt.Errorf("eof inside <array>")
			}
			if string(tag) == "</array>" {
				data = rest
				break
			}
			elem := reflect.New(t.Elem()).Elem()
			data, err = unmarshalValue(data, elem)
			if err != nil {
				return nil, err
			}
			v.Set(reflect.Append(v, elem))
		}
		return data, nil

	case "<string>":
		body, etag, data := next(data)
		if len(etag) == 0 {
			return nil, fmt.Errorf("eof inside <string>")
		}
		if string(etag) != "</string>" {
			return nil, fmt.Errorf("expected </string> but got %s", etag)
		}
		// TODO: unescape
		v.Set(reflect.ValueOf(string(body)))
		return data, nil

	case "<integer>":
		body, etag, data := next(data)
		if len(etag) == 0 {
			return nil, fmt.Errorf("eof inside <integer>")
		}
		if string(etag) != "</integer>" {
			return nil, fmt.Errorf("expected </integer> but got %s", etag)
		}
		i, err := strconv.Atoi(string(body))
		if err != nil {
			return nil, fmt.Errorf("non-integer in <integer> tag: %s", body)
		}
		v.Set(reflect.ValueOf(i))
		return data, nil
	case "<real>":
		bits := 64
		if v.Kind() == reflect.Float32 {
			bits = 32
		}
		body, etag, data := next(data)
		if len(etag) == 0 {
			return nil, fmt.Errorf("eof inside <real>")
		}
		if string(etag) != "</real>" {
			return nil, fmt.Errorf("expected </real> but got %s", etag)
		}
		f, err := strconv.ParseFloat(string(body), bits)
		if err != nil {
			return nil, fmt.Errorf("non-float in <real> tag: %s", body)
		}
		v.Set(reflect.ValueOf(f))
		return data, nil
	case "<date>":
		body, etag, data := next(data)
		if len(etag) == 0 {
			return nil, fmt.Errorf("eof inside <date>")
		}
		if string(etag) != "</date>" {
			return nil, fmt.Errorf("expected </date> but got %s", etag)
		}
		t, err := time.Parse(time.RFC3339, string(body))
		if err != nil {
			return nil, fmt.Errorf("non-date in <date> tag: %s", body)
		}
		v.Set(reflect.ValueOf(t))
		return data, nil
	case "<data>":
		body, etag, data := next(data)
		if len(etag) == 0 {
			return nil, fmt.Errorf("eof inside <data>")
		}
		if string(etag) != "</data>" {
			return nil, fmt.Errorf("expected </data> but got %s", etag)
		}
		d, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			return nil, fmt.Errorf("non-base64 in <data> tag: %s", body)
		}
		v.Set(reflect.ValueOf(d))
		return data, nil
	case "<true/>":
		b := true
		v.Set(reflect.ValueOf(b))
		return data, nil
	case "<false/>":
		b := false
		v.Set(reflect.ValueOf(b))
		return data, nil
	}
	return nil, fmt.Errorf("unexpected tag %s", tag)
}

func skipValue(data []byte) (rest []byte, err error) {
	n := 0
	for {
		var tag []byte
		_, tag, data = next(data)
		if len(tag) == 0 {
			return nil, fmt.Errorf("unexpected eof")
		}
		if tag[1] == '/' {
			if n == 0 {
				return nil, fmt.Errorf("unexpected closing tag")
			}
			n--
			if n == 0 {
				break
			}
		} else {
			n++
		}
	}
	return data, nil
}
