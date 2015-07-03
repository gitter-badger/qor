package exchange

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/now"
	"github.com/qor/qor"
	"github.com/qor/qor/resource"
	"github.com/qor/qor/roles"
	"github.com/qor/qor/utils"
)

type Meta struct {
	base *Resource
	Name string
	// Alias string
	Label string
	// Type          string
	Valuer   func(interface{}, *qor.Context) interface{}
	Setter   func(resource interface{}, metaValue *resource.MetaValue, context *qor.Context)
	Metas    []resource.Metaor
	Resource resource.Resourcer
	// Collection    interface{}
	// GetCollection func(interface{}, *qor.Context) [][]string
	Permission *roles.Permission

	Optional     bool
	AliasHeaders []string
}

func (meta *Meta) GetName() string {
	return meta.Name
}

func (meta *Meta) GetFieldName() string {
	return meta.Name
}

func (meta *Meta) GetMetas() []resource.Metaor {
	if len(meta.Metas) > 0 {
		return meta.Metas
	} else if meta.Resource == nil {
		return []resource.Metaor{}
	} else {
		return meta.Resource.GetMetas()
	}
}

func (meta *Meta) GetResource() resource.Resourcer {
	return meta.Resource
}

func (meta *Meta) GetValuer() func(interface{}, *qor.Context) interface{} {
	return meta.Valuer
}

func (meta *Meta) GetSetter() func(resource interface{}, metaValue *resource.MetaValue, context *qor.Context) {
	return meta.Setter
}

func (meta *Meta) HasPermission(mode roles.PermissionMode, context *qor.Context) bool {
	if meta.Permission == nil {
		return true
	}
	return meta.Permission.HasPermission(mode, context.Roles...)
}

func (m *Meta) Set(field string, val interface{}) *Meta {
	reflect.ValueOf(m).Elem().FieldByName(field).Set(reflect.ValueOf(val))
	return m
}

func (m *Meta) getCurrentLabel(vmap map[string]string, index int) string {
	var labels []string
	if index > 0 {
		// support both "label 01" and "label 1"
		labels = append(labels, fmt.Sprintf("%s %02d", m.Label, index), fmt.Sprintf("%s %d", m.Label, index))
	} else {
		labels = append(labels, m.Label)
	}

	labels = append(labels, m.AliasHeaders...)
	for _, label := range labels {
		if _, ok := vmap[label]; ok {
			return label
		}
	}

	return ""
}

func (meta *Meta) updateMeta() {
	if meta.Name == "" {
		qor.ExitWithMsg("Meta should have name: %v", reflect.ValueOf(meta).Type())
		// } else if meta.Alias == "" {
		// 	meta.Alias = meta.Name
	}

	if meta.Label == "" {
		meta.Label = utils.HumanizeString(meta.Name)
	}

	var (
		scope       = &gorm.Scope{Value: meta.base.Value}
		nestedField = strings.Contains(meta.Name, ".")
		field       *gorm.Field
		hasColumn   bool
		valueType   string
	)

	if nestedField {
		subModel, name := utils.ParseNestedField(reflect.ValueOf(meta.base.Value), meta.Name)
		subScope := &gorm.Scope{Value: subModel.Interface()}
		field, hasColumn = utils.GetField(subScope.Fields(), name)
	} else {
		if field, hasColumn = utils.GetField(scope.Fields(), meta.Name); hasColumn {
			meta.Name = field.Name
		}
	}

	if hasColumn {
		ft := field.Field.Type()
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		valueType = ft.Kind().String()
	}

	// Set Meta Resource
	if meta.Resource == nil {
		if hasColumn && (field.Relationship != nil) {
			var result interface{}
			if valueType == "struct" {
				result = reflect.New(field.Field.Type()).Interface()
			} else if valueType == "slice" {
				result = reflect.New(field.Field.Type().Elem()).Interface()
			}
			newRes := &Resource{}
			newRes.Value = result
			meta.Resource = newRes
		}
	}

	// Set Meta Value
	if meta.Valuer == nil {
		if hasColumn {
			meta.Valuer = func(value interface{}, context *qor.Context) interface{} {
				scope := &gorm.Scope{Value: value}
				alias := meta.Name
				if nestedField {
					fields := strings.Split(alias, ".")
					alias = fields[len(fields)-1]
				}

				if f, ok := scope.FieldByName(alias); ok {
					if field.Relationship != nil {
						if f.Field.CanAddr() {
							context.GetDB().Model(value).Related(f.Field.Addr().Interface(), meta.Name)
						}
					}
					if f.Field.CanAddr() {
						return f.Field.Addr().Interface()
					} else {
						return f.Field.Interface()
					}
				}

				return ""
			}
		} else {
			// qor.ExitWithMsg("Unsupported meta name %v for resource %v", meta.Name, reflect.TypeOf(base.Value))
			fmt.Printf("Unsupported meta name %v for resource %T\n", meta.Name, meta.base.Value)
			debug.PrintStack()
		}
	}

	scopeField, _ := scope.FieldByName(meta.Name)

	if meta.Setter == nil {
		meta.Setter = func(resource interface{}, metaValue *resource.MetaValue, context *qor.Context) {
			if metaValue == nil {
				return
			}

			value := metaValue.Value
			scope := &gorm.Scope{Value: resource}
			alias := meta.Name
			if nestedField {
				fields := strings.Split(alias, ".")
				alias = fields[len(fields)-1]
			}
			field := reflect.Indirect(reflect.ValueOf(resource)).FieldByName(alias)
			if field.Kind() == reflect.Ptr && field.IsNil() {
				field.Set(utils.NewValue(field.Type()).Elem())
			}
			for field.Kind() == reflect.Ptr {
				field = field.Elem()
			}

			if field.IsValid() && field.CanAddr() {
				var relationship string
				if scopeField != nil && scopeField.Relationship != nil {
					relationship = scopeField.Relationship.Kind
				}
				if relationship == "many_to_many" {
					context.GetDB().Where(utils.ToArray(value)).Find(field.Addr().Interface())
					if !scope.PrimaryKeyZero() {
						context.GetDB().Model(resource).Association(meta.Name).Replace(field.Interface())
					}
				} else {
					switch field.Kind() {
					case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
						field.SetInt(utils.ToInt(value))
					case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
						field.SetUint(utils.ToUint(value))
					case reflect.Float32, reflect.Float64:
						field.SetFloat(utils.ToFloat(value))
					default:
						if scanner, ok := field.Addr().Interface().(sql.Scanner); ok {
							if scanner.Scan(value) != nil {
								scanner.Scan(utils.ToString(value))
							}
						} else if reflect.TypeOf("").ConvertibleTo(field.Type()) {
							field.Set(reflect.ValueOf(utils.ToString(value)).Convert(field.Type()))
						} else if reflect.TypeOf([]string{}).ConvertibleTo(field.Type()) {
							field.Set(reflect.ValueOf(utils.ToArray(value)).Convert(field.Type()))
						} else if rvalue := reflect.ValueOf(value); reflect.TypeOf(rvalue.Type()).ConvertibleTo(field.Type()) {
							field.Set(rvalue.Convert(field.Type()))
						} else if _, ok := field.Addr().Interface().(*time.Time); ok {
							if str := utils.ToString(value); str != "" {
								if newTime, err := now.Parse(str); err == nil {
									field.Set(reflect.ValueOf(newTime))
								}
							}
						} else {
							var buf = bytes.NewBufferString("")
							json.NewEncoder(buf).Encode(value)
							if err := json.NewDecoder(strings.NewReader(buf.String())).Decode(field.Addr().Interface()); err != nil {
								// TODO: should not kill the process
								qor.ExitWithMsg("Can't set value %v to %v [meta %v]", reflect.ValueOf(value).Type(), field.Type(), meta)
							}
						}
					}
				}
			}
		}
	}

	if nestedField {
		oldvalue := meta.Valuer
		meta.Valuer = func(value interface{}, context *qor.Context) interface{} {
			return oldvalue(utils.GetNestedModel(value, meta.Name, context), context)
		}
		oldSetter := meta.Setter
		meta.Setter = func(resource interface{}, metaValue *resource.MetaValue, context *qor.Context) {
			oldSetter(utils.GetNestedModel(resource, meta.Name, context), metaValue, context)
		}
	}
}
