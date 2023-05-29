package file

import (
	"bufio"
	"errors"
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

func (s *Source) Load(c *config.Config) (e error) {
	fh, err := os.Open(s.filepath)
	if err != nil {
		return err
	}
	defer fh.Close()

	// only ini suported
	if err = parseIni(fh, c); err != nil {
		return err
	}

	return nil
}

func parseIni(r io.Reader, cfg *config.Config) (e error) {
	elem := reflect.ValueOf(cfg).Elem()
	t := elem.Type()

	var currentSection *reflect.Value

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
					currentSection = &a
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
			el := currentSection.Elem()
			if err := parseParam(&el, nil, strings.TrimSpace(key), strings.TrimSpace(value)); err != nil {
				return err
			}
		}
	}

	if err := s.Err(); err != nil {
		return err
	}

	return nil
}

func parseParam(currentSection *reflect.Value, subscribtionRoot config.Propagator, key, value string) error {
	key, rest, _ := strings.Cut(key, ".")

	sRoot := subscribtionRoot
	t := currentSection.Type()
	for j := 0; j < t.NumMethod(); j++ {
		m := t.Method(j)
		if m.Name == "Propagate" {
			sRoot = currentSection.Interface().(config.Propagator)
		}
	}

	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if name, ok := f.Tag.Lookup("config"); ok && name == key {
			v := strings.TrimSpace(value)
			el := currentSection.FieldByName(f.Name)

			switch el.Interface().(type) {
			case time.Duration:
				b, err := paramParseTimeDuration(v)
				if err != nil {
					return err
				}
				if sRoot != nil {
					sRoot.Propagate(structs.OldNew{
						Name: f.Name, // or maybe `name`
						New:  el,
						Old:  b,
					})
				}
				el.Set(reflect.ValueOf(b))
				continue
			default:
			}

			switch f.Type.Kind() {
			case reflect.Pointer:
				v := el.Elem()
				if err := parseParam(&v, sRoot, rest, value); err != nil {
					return err
				}

			case reflect.Bool:
				b, err := paramParseBool(v)
				if err != nil {
					return err
				}
				if el.Bool() != b {
					log.Println("different value, setting b ", b)
					if sRoot != nil {
						sRoot.Propagate(structs.OldNew{
							Name: f.Name,
							New:  b,
							Old:  el.Bool(),
						})
					}
					el.SetBool(b)
				}
			case reflect.Uint:
				uintP, err := paramParseUint(v)
				if err != nil {
					return err
				}
				if el.Uint() != uintP {
					log.Println("different value, setting ui ", uintP)
					if sRoot != nil {
						sRoot.Propagate(structs.OldNew{
							Name: f.Name,
							New:  uintP,
							Old:  el.Uint(),
						})
					}
					el.SetUint(uintP)
				}
			case reflect.Int:
				intP, err := paramParseInt(v)
				if err != nil {
					return err
				}
				if el.Int() != intP {
					log.Println("different value, setting i ", intP)
					if sRoot != nil {
						sRoot.Propagate(structs.OldNew{
							Name: f.Name,
							New:  intP,
							Old:  el.Int(),
						})
					}
					el.SetInt(intP)
				}
			case reflect.String:
				s, err := paramParseString(v)
				if err != nil {
					return err
				}
				if el.String() != s {
					log.Println("different value, setting s ", s)
					if sRoot != nil {
						sRoot.Propagate(structs.OldNew{
							Name: f.Name,
							New:  s,
							Old:  el.String(),
						})
					}
					el.SetString(s)
				}
			case reflect.Struct:
				err := parseParam(&el, sRoot, rest, value)
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
			log.Println("key", key, value, currentSection.Kind(), el)
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

func paramParseTimeDuration(value string) (time.Duration, error) {
	return time.ParseDuration(value)
}

func paramParseInt(value string) (int64, error) {
	return strconv.ParseInt(value, 10, 64)
}

func paramParseUint(value string) (uint64, error) {
	return strconv.ParseUint(value, 10, 64)
}
