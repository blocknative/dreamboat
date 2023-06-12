package file

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/blocknative/dreamboat/cmd/dreamboat/config"
	"github.com/blocknative/dreamboat/structs"
)

type Source struct {
	filepath string
}

func NewSource(filepath string) (s *Source) {
	return &Source{
		filepath: filepath,
	}
}

func (s *Source) Load(c *config.Config, initial bool) (e error) {
	fh, err := os.Open(s.filepath)
	if err != nil {
		return err
	}
	defer fh.Close()

	// only ini suported
	if err = parseIni(fh, c, initial); err != nil {
		return err
	}

	return nil
}

func parseIni(r io.Reader, cfg *config.Config, initial bool) (e error) {
	elem := reflect.ValueOf(cfg).Elem()
	t := elem.Type()

	var (
		currentSection *reflect.Value
		currentTag     string
		sRoot          config.Propagator
	)

	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if len(line) < 1 {
			continue
		}

		switch line[0] {
		case '#', '/', ';':
			// dissregard any comments
		case '[': // section
			tag, _, ok := strings.Cut(line[1:], "]")
			if !ok {
				return errors.New("parse failure")
			}
			tag = strings.TrimSpace(tag)
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if name, ok := f.Tag.Lookup("config"); ok && name == tag {
					a := elem.FieldByName(f.Name)
					currentTag = tag
					currentSection = &a
					/*
						for j := 0; j < currentSection.NumMethod(); j++ {
							m := t.Method(j)
							if m.Name == "Propagate" {
								sRoot = currentSection.Interface().(config.Propagator)
							}
						}*/
					continue
				}
			}
		default:
			if len(line) == 0 {
				continue
			}
			key, value, found := strings.Cut(line, "=")
			if !found {
				return errors.New("parse failure")
			}

			if currentSection == nil {
				return errors.New("malformed config")
			}

			if currentSection.Kind() == reflect.Ptr {
				if !currentSection.IsNil() {
					a := currentSection.Elem()

					if err := parseParam(&a, sRoot, currentTag+"."+strings.TrimSpace(key), strings.TrimSpace(key), strings.TrimSpace(value), initial); err != nil {
						return err
					}
				} else {
					if err := parseParam(currentSection, sRoot, currentTag+"."+strings.TrimSpace(key), strings.TrimSpace(key), strings.TrimSpace(value), initial); err != nil {
						return err
					}
				}
				continue
			}

			if currentSection.Kind() == reflect.Struct {
				a := currentSection.Elem()
				if err := parseParam(&a, sRoot, currentTag+"."+strings.TrimSpace(key), strings.TrimSpace(key), strings.TrimSpace(value), initial); err != nil {
					return err
				}
				continue
			}

			return errors.New("malformed config")
		}
	}

	if err := s.Err(); err != nil {
		return err
	}

	return nil
}

func parseParam(currentSection *reflect.Value, subscribtionRoot config.Propagator, fullPath, key, value string, initial bool) error {
	key, rest, _ := strings.Cut(key, ".")

	sRoot := subscribtionRoot
	/*
		if currentSection.Kind() == reflect.Ptr {
			a := currentSection.Elem()
			currentSection = &a
		}*/
	t := currentSection.Type()
	for j := 0; j < t.NumMethod(); j++ {
		m := t.Method(j)
		if m.Name == "Propagate" {
			sRoot = currentSection.Interface().(config.Propagator)
		}
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if v, ok := f.Tag.Lookup("config"); ok {
			params := strings.Split(v, ",")
			if params[0] != key {
				continue
			}

			v := strings.TrimSpace(value)
			el := currentSection.FieldByName(f.Name)

			switch el.Interface().(type) {
			case time.Duration:
				b, err := paramParseTimeDuration(v)
				if err != nil {
					return err
				}

				if !initial && !inArr(params, "allow_dynamic") {
					return fmt.Errorf("dynamic change of %s parameter is not allowed  ", key)
				}
				if sRoot != nil {
					sRoot.Propagate(structs.OldNew{
						Name:      f.Name, // or maybe `name`
						ParamPath: key,
						New:       el,
						Old:       b,
					})
				}
				el.Set(reflect.ValueOf(b))
				continue
			default:
			}

			switch f.Type.Kind() {
			case reflect.Pointer:
				v := el.Elem()
				if err := parseParam(&v, sRoot, fullPath, rest, value, initial); err != nil {
					return err
				}

			case reflect.Bool:
				b, err := paramParseBool(v)
				if err != nil {
					return err
				}
				if el.Bool() != b {
					if !initial {
						//		log.Println("different value, setting bool ", b)
						if !inArr(params, "allow_dynamic") {
							return fmt.Errorf("dynamic change of %s parameter is not allowed  ", key)
						}
						if sRoot != nil {
							sRoot.Propagate(structs.OldNew{
								Name:      f.Name,
								ParamPath: key,
								New:       b,
								Old:       el.Bool(),
							})
						}
					}
					el.SetBool(b)
				}
			case reflect.Uint:
				uintP, err := paramParseUint(v)
				if err != nil {
					return err
				}
				if el.Uint() != uintP {
					if !initial {
						//	log.Println("different value, setting ui ", uintP)

						if !inArr(params, "allow_dynamic") {
							return fmt.Errorf("dynamic change of %s parameter is not allowed  ", key)
						}

						if sRoot != nil {
							sRoot.Propagate(structs.OldNew{
								Name:      f.Name,
								ParamPath: key,
								New:       uintP,
								Old:       el.Uint(),
							})
						}
					}
					el.SetUint(uintP)
				}
			case reflect.Int:
				intP, err := paramParseInt(v)
				if err != nil {
					return err
				}
				if el.Int() != intP {
					if !initial {
						if !inArr(params, "allow_dynamic") {
							return fmt.Errorf("dynamic change of %s parameter is not allowed  ", key)
						}

						//	log.Println("different value, setting i ", intP)
						if sRoot != nil {
							sRoot.Propagate(structs.OldNew{
								Name:      f.Name,
								ParamPath: key,
								New:       intP,
								Old:       el.Int(),
							})
						}
					}
					el.SetInt(intP)
				}
			case reflect.String:
				s, err := paramParseString(v)
				if err != nil {
					return err
				}
				if el.String() != s {
					if !initial {
						if !inArr(params, "allow_dynamic") {
							return fmt.Errorf("dynamic change of %s parameter is not allowed  ", key)
						}

						//	log.Println("different value, setting s ", s)
						if sRoot != nil {
							sRoot.Propagate(structs.OldNew{
								Name:      f.Name,
								ParamPath: key,
								New:       s,
								Old:       el.String(),
							})
						}
					}
					el.SetString(s)
				}
			case reflect.Slice:
				switch f.Type.Elem().Kind() {
				case reflect.String:
					s, err := paramParseStringSlice(v)
					if err != nil {
						return err
					}

					a := el.Interface().([]string)
					if !reflect.DeepEqual(s, a) {
						if !initial {
							if !inArr(params, "allow_dynamic") {
								return fmt.Errorf("dynamic change of %s parameter is not allowed  ", key)
							}
							if sRoot != nil {
								sRoot.Propagate(structs.OldNew{
									Name:      f.Name,
									ParamPath: key,
									New:       s,
									Old:       el,
								})
							}
						}
						el.Set(reflect.ValueOf(s))
					}
				default:
					log.Panic("unsupported slice type: ", f.Type.Elem().Kind())
				}
			case reflect.Struct:
				err := parseParam(&el, sRoot, fullPath, rest, value, initial)
				if err != nil {
					return err
				}

			default:
				if !el.CanAddr() {
					log.Panic("unsupported type ", currentSection.Elem().Kind())
				} else {
					log.Panic("unsupported type ", currentSection.Kind())
				}

			}
			//	log.Println("key", key, value, currentSection.Kind(), el)
		}
	}
	return nil

}

func paramParseBool(value string) (bool, error) {
	s := strings.ToLower(value)
	if s == "true" || s == "1" {
		return true, nil
	}

	return false, nil
}

func paramParseString(value string) (string, error) {
	return value, nil
}

func paramParseStringSlice(value string) ([]string, error) {
	return strings.Split(value, ","), nil
}

func paramParseTimeDuration(value string) (time.Duration, error) {
	return time.ParseDuration(value)
}

func paramParseInt(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func paramParseUint(value string) (uint64, error) {
	return strconv.ParseUint(value, 10, 64)
}

func inArr(in []string, b string) bool {
	for _, a := range in {
		if a == b {
			return true
		}
	}
	return false
}
