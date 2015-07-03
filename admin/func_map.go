package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"os"
	"path"
	"reflect"
	"runtime/debug"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/qor/qor"
	"github.com/qor/qor/roles"
	"github.com/qor/qor/utils"
	"github.com/theplant/cldr"
)

func (context *Context) NewResourceContext(name ...string) *Context {
	clone := &Context{Context: context.Context, Admin: context.Admin, Result: context.Result}
	if len(name) > 0 {
		clone.SetResource(context.Admin.GetResource(name[0]))
	} else {
		clone.SetResource(context.Resource)
	}
	return clone
}

func (context *Context) PrimaryKeyOf(value interface{}) interface{} {
	return context.GetDB().NewScope(value).PrimaryKeyValue()
}

func (context *Context) IsNewRecord(value interface{}) bool {
	return context.GetDB().NewRecord(value)
}

func (context *Context) ValueOf(value interface{}, meta *Meta) interface{} {
	result := meta.Valuer(value, context.Context)

	if reflectValue := reflect.ValueOf(result); reflectValue.IsValid() {
		if reflectValue.Kind() == reflect.Ptr {
			if reflectValue.IsNil() || !reflectValue.Elem().IsValid() {
				return nil
			}

			result = reflectValue.Elem().Interface()
		}

		if !(meta.Type == "collection_edit" || meta.Type == "single_edit") {
			if context.IsNewRecord(value) && reflect.DeepEqual(reflect.Zero(reflect.TypeOf(result)).Interface(), result) {
				return nil
			}
		}
		return result
	}

	return nil
}

func (context *Context) NewResourcePath(value interface{}) string {
	if res, ok := value.(*Resource); ok {
		return path.Join(context.Admin.router.Prefix, res.ToParam(), "new")
	} else {
		return path.Join(context.Admin.router.Prefix, context.Resource.ToParam(), "new")
	}
}

func (context *Context) UrlFor(value interface{}, resources ...*Resource) string {
	var url string
	if admin, ok := value.(*Admin); ok {
		url = admin.router.Prefix
	} else if res, ok := value.(*Resource); ok {
		url = path.Join(context.Admin.router.Prefix, res.ToParam())
	} else {
		structType := reflect.Indirect(reflect.ValueOf(value)).Type().String()
		res := context.Admin.GetResource(structType)
		if res == nil {
			url = ""
		} else {
			primaryKey := context.GetDB().NewScope(value).PrimaryKeyValue()
			url = path.Join(context.Admin.router.Prefix, res.ToParam(), fmt.Sprintf("%v", primaryKey))
		}
	}
	return url
}

func (context *Context) LinkTo(text interface{}, link interface{}) template.HTML {
	text = reflect.Indirect(reflect.ValueOf(text)).Interface()
	if linkStr, ok := link.(string); ok {
		return template.HTML(fmt.Sprintf(`<a href="%v">%v</a>`, linkStr, text))
	}
	return template.HTML(fmt.Sprintf(`<a href="%v">%v</a>`, context.UrlFor(link), text))
}

func (context *Context) RenderIndex(value interface{}, meta *Meta) template.HTML {
	var err error
	var result = bytes.NewBufferString("")
	var tmpl = template.New(meta.Type + ".tmpl").Funcs(context.funcMap())

	if tmpl, err = context.findTemplate(tmpl, fmt.Sprintf("metas/index/%v.tmpl", meta.Type)); err != nil {
		tmpl, _ = tmpl.Parse("{{.Value}}")
	}

	data := map[string]interface{}{"Value": context.ValueOf(value, meta), "Meta": meta}
	if err := tmpl.Execute(result, data); err != nil {
		fmt.Println(err)
		debug.PrintStack()
	}
	return template.HTML(result.String())
}

func (context *Context) RenderForm(value interface{}, metas []*Meta) template.HTML {
	var result = bytes.NewBufferString("")
	context.renderForm(result, value, metas, []string{"QorResource"})
	return template.HTML(result.String())
}

func (context *Context) renderForm(result *bytes.Buffer, value interface{}, metas []*Meta, prefix []string) {
	for _, meta := range metas {
		context.RenderMeta(result, meta, value, prefix)
	}
}

func (context *Context) RenderMeta(writer *bytes.Buffer, meta *Meta, value interface{}, prefix []string) {
	prefix = append(prefix, meta.Name)

	funcsMap := context.funcMap()
	funcsMap["render_form"] = func(value interface{}, metas []*Meta, index ...int) template.HTML {
		var result = bytes.NewBufferString("")
		newPrefix := append([]string{}, prefix...)

		if len(index) > 0 {
			last := newPrefix[len(newPrefix)-1]
			newPrefix = append(newPrefix[:len(newPrefix)-1], fmt.Sprintf("%v[%v]", last, index[0]))
		}

		context.renderForm(result, value, metas, newPrefix)
		return template.HTML(result.String())
	}

	var tmpl = template.New(meta.Type + ".tmpl").Funcs(funcsMap)

	if tmpl, err := context.findTemplate(tmpl, fmt.Sprintf("metas/form/%v.tmpl", meta.Type)); err == nil {
		data := map[string]interface{}{}
		data["Base"] = meta.base
		data["InputId"] = strings.Join(prefix, "")
		data["Label"] = meta.Label
		data["InputName"] = strings.Join(prefix, ".")
		data["Value"] = context.ValueOf(value, meta)

		if meta.GetCollection != nil {
			data["CollectionValue"] = meta.GetCollection(value, context.Context)
		}

		data["Meta"] = meta

		if err := tmpl.Execute(writer, data); err != nil {
			panic(err)
		}
	} else {
		panic(fmt.Sprintf("%v: form type %v not supported", meta.Name, meta.Type))
	}
}

func (context *Context) IsIncluded(value interface{}, primaryKey interface{}) bool {
	primaryKeys := []interface{}{}
	reflectValue := reflect.Indirect(reflect.ValueOf(value))
	if reflectValue.Kind() == reflect.Slice {
		for i := 0; i < reflectValue.Len(); i++ {
			if value := reflectValue.Index(i); value.IsValid() {
				if reflect.Indirect(value).Kind() == reflect.Struct {
					scope := &gorm.Scope{Value: reflectValue.Index(i).Interface()}
					primaryKeys = append(primaryKeys, scope.PrimaryKeyValue())
				} else {
					primaryKeys = append(primaryKeys, reflect.Indirect(reflectValue.Index(i)).Interface())
				}
			}
		}
	} else if reflectValue.Kind() == reflect.Struct {
		scope := &gorm.Scope{Value: value}
		primaryKeys = append(primaryKeys, scope.PrimaryKeyValue())
	} else {
		if reflectValue.IsValid() {
			primaryKeys = append(primaryKeys, reflect.Indirect(reflectValue).Interface())
		}
	}

	for _, key := range primaryKeys {
		if fmt.Sprintf("%v", primaryKey) == fmt.Sprintf("%v", key) {
			return true
		}
	}
	return false
}

func (context *Context) getResource(resources ...*Resource) *Resource {
	for _, res := range resources {
		return res
	}
	return context.Resource
}

func (context *Context) AllMetas(resources ...*Resource) []*Meta {
	return context.getResource(resources...).AllMetas()
}

func (context *Context) IndexMetas(resources ...*Resource) []*Meta {
	res := context.getResource(resources...)
	return res.AllowedMetas(res.IndexMetas(), context, roles.Read)
}

func (context *Context) EditMetas(resources ...*Resource) []*Meta {
	res := context.getResource(resources...)
	return res.AllowedMetas(res.EditMetas(), context, roles.Update)
}

func (context *Context) ShowMetas(resources ...*Resource) []*Meta {
	res := context.getResource(resources...)
	return res.AllowedMetas(res.ShowMetas(), context, roles.Read)
}

func (context *Context) NewMetas(resources ...*Resource) []*Meta {
	res := context.getResource(resources...)
	return res.AllowedMetas(res.NewMetas(), context, roles.Create)
}

func (context *Context) JavaScriptTag(name string) template.HTML {
	name = path.Join(context.Admin.GetRouter().Prefix, "assets", "javascripts", name+".js")
	return template.HTML(fmt.Sprintf(`<script src="%s"></script>`, name))
}

func (context *Context) StyleSheetTag(name string) template.HTML {
	name = path.Join(context.Admin.GetRouter().Prefix, "assets", "stylesheets", name+".css")
	return template.HTML(fmt.Sprintf(`<link type="text/css" rel="stylesheet" href="%s">`, name))
}

type scopeMenu struct {
	Group  string
	Scopes []*Scope
}

func (context *Context) GetScopes() (menus []*scopeMenu) {
OUT:
	for _, scope := range context.Resource.scopes {
		if !scope.Default {
			if scope.Group != "" {
				for _, menu := range menus {
					if menu.Group == scope.Group {
						menu.Scopes = append(menu.Scopes, scope)
						continue OUT
					}
				}
				menus = append(menus, &scopeMenu{Group: scope.Group, Scopes: []*Scope{scope}})
			} else {
				menus = append(menus, &scopeMenu{Group: scope.Group, Scopes: []*Scope{scope}})
			}
		}
	}
	return menus
}

type permissioner interface {
	HasPermission(roles.PermissionMode, *qor.Context) bool
}

func (context *Context) HasCreatePermission(permissioner permissioner) bool {
	return permissioner.HasPermission(roles.Create, context.GetContext())
}

func (context *Context) HasReadPermission(permissioner permissioner) bool {
	return permissioner.HasPermission(roles.Read, context.GetContext())
}

func (context *Context) HasUpdatePermission(permissioner permissioner) bool {
	return permissioner.HasPermission(roles.Update, context.GetContext())
}

func (context *Context) HasDeletePermission(permissioner permissioner) bool {
	return permissioner.HasPermission(roles.Delete, context.GetContext())
}

type Page struct {
	Page       int
	Current    bool
	IsPrevious bool
	IsNext     bool
}

const (
	VISIBLE_PAGE_COUNT = 8
)

// Keep VISIBLE_PAGE_COUNT's pages visible, exclude prev and next link
// Assume there are 12 pages in total.
// When current page is 1
// [current, 2, 3, 4, 5, 6, 7, 8, next]
// When current page is 6
// [prev, 2, 3, 4, 5, current, 7, 8, 9, 10, next]
// When current page is 10
// [prev, 5, 6, 7, 8, 9, current, 11, 12]
// If total page count less than VISIBLE_PAGE_COUNT, always show all pages
func (context *Context) Pagination() *[]Page {
	pagination := context.Searcher.Pagination
	if pagination.Pages <= 1 {
		return nil
	}

	start := pagination.CurrentPage - VISIBLE_PAGE_COUNT/2
	if start < 1 {
		start = 1
	}

	end := start + VISIBLE_PAGE_COUNT - 1 // -1 for "start page" itself
	if end > pagination.Pages {
		end = pagination.Pages
	}

	if (end-start) < VISIBLE_PAGE_COUNT && start != 1 {
		start = end - VISIBLE_PAGE_COUNT + 1
	}
	if start < 1 {
		start = 1
	}

	var pages []Page
	// Append prev link
	if start > 1 {
		pages = append(pages, Page{Page: start - 1, IsPrevious: true})
	}

	for i := start; i <= end; i++ {
		pages = append(pages, Page{Page: i, Current: pagination.CurrentPage == i})
	}

	// Append next link
	if end < pagination.Pages {
		pages = append(pages, Page{Page: end + 1, IsNext: true})
	}

	return &pages
}

func Equal(a, b interface{}) bool {
	return reflect.DeepEqual(a, b)
}

// PatchCurrentURL is a convinent wrapper for qor/utils.PatchURL
func (context *Context) PatchCurrentURL(params ...interface{}) (patchedURL string, err error) {
	return utils.PatchURL(context.Request.URL.String(), params...)
}

// PatchURL is a convinent wrapper for qor/utils.PatchURL
func (context *Context) PatchURL(url string, params ...interface{}) (patchedURL string, err error) {
	return utils.PatchURL(url, params...)
}

func (context *Context) themesClass() (result string) {
	var results []string
	if context.Resource != nil {
		for _, theme := range context.Resource.Config.Themes {
			results = append(results, "qor-theme-"+theme)
		}
	}
	return strings.Join(results, " ")
}

func (context *Context) LoadThemeStyleSheets() template.HTML {
	var results []string
	if context.Resource != nil {
		for _, theme := range context.Resource.Config.Themes {
			for _, view := range context.getViewPaths() {
				file := path.Join("assets", "stylesheets", theme+".css")
				if _, err := os.Stat(path.Join(view, file)); err == nil {
					results = append(results, fmt.Sprintf(`<link type="text/css" rel="stylesheet" href="%s?theme=%s">`, path.Join(context.Admin.GetRouter().Prefix, file), theme))
					break
				}
			}
		}
	}
	return template.HTML(strings.Join(results, " "))
}

func (context *Context) LoadThemeJavaScripts() template.HTML {
	var results []string
	if context.Resource != nil {
		for _, theme := range context.Resource.Config.Themes {
			for _, view := range context.getViewPaths() {
				file := path.Join("assets", "javascripts", theme+".js")
				if _, err := os.Stat(path.Join(view, file)); err == nil {
					results = append(results, fmt.Sprintf(`<script src="%s?theme=%s"></script>`, path.Join(context.Admin.GetRouter().Prefix, file), theme))
					break
				}
			}
		}
	}
	return template.HTML(strings.Join(results, " "))
}

func (context *Context) LogoutURL() string {
	if context.Admin.auth != nil {
		return context.Admin.auth.LogoutURL(context)
	}
	return ""
}

func (context *Context) T(key string, values ...interface{}) string {
	locale := utils.GetLocale(context.GetContext())

	if context.Admin.I18n == nil {
		if result, err := cldr.Parse(locale, key, values...); err == nil {
			return result
		}
		return key
	} else {
		return context.Admin.I18n.Scope("qor_admin").T(locale, key, values...)
	}
}

func (context *Context) RT(resource *Resource, key string, values ...interface{}) string {
	locale := utils.GetLocale(context.GetContext())

	if context.Admin.I18n == nil {
		if result, err := cldr.Parse(locale, key, values); err == nil {
			return result
		}
		return key
	} else {
		return context.Admin.I18n.Scope(strings.Join([]string{"qor_admin", resource.ToParam()}, ".")).T(locale, key, values...)
	}
}

func (context *Context) funcMap() template.FuncMap {
	funcMap := template.FuncMap{
		"equal": Equal,

		"current_user":         func() qor.CurrentUser { return context.CurrentUser },
		"get_resource":         context.GetResource,
		"new_resource_context": context.NewResourceContext,
		"is_new_record":        context.IsNewRecord,
		"is_included":          context.IsIncluded,
		"primary_key_of":       context.PrimaryKeyOf,
		"value_of":             context.ValueOf,

		"menus":      context.Admin.GetMenus,
		"get_scopes": context.GetScopes,

		"escape":                 html.EscapeString,
		"raw":                    func(str string) template.HTML { return template.HTML(str) },
		"stringify":              utils.Stringify,
		"render":                 context.Render,
		"render_form":            context.RenderForm,
		"render_index":           context.RenderIndex,
		"url_for":                context.UrlFor,
		"link_to":                context.LinkTo,
		"patch_current_url":      context.PatchCurrentURL,
		"patch_url":              context.PatchURL,
		"new_resource_path":      context.NewResourcePath,
		"qor_theme_class":        context.themesClass,
		"javascript_tag":         context.JavaScriptTag,
		"stylesheet_tag":         context.StyleSheetTag,
		"load_theme_stylesheets": context.LoadThemeStyleSheets,
		"load_theme_javascripts": context.LoadThemeJavaScripts,
		"pagination":             context.Pagination,

		"all_metas":   context.AllMetas,
		"index_metas": context.IndexMetas,
		"edit_metas":  context.EditMetas,
		"show_metas":  context.ShowMetas,
		"new_metas":   context.NewMetas,

		"has_create_permission": context.HasCreatePermission,
		"has_read_permission":   context.HasReadPermission,
		"has_update_permission": context.HasUpdatePermission,
		"has_delete_permission": context.HasDeletePermission,

		"logout_url": context.LogoutURL,

		"marshal": func(v interface{}) template.JS {
			a, _ := json.Marshal(v)
			return template.JS(a)
		},

		"t":       context.T,
		"rt":      context.RT,
		"flashes": context.GetFlashes,
	}

	for key, value := range context.Admin.funcMaps {
		funcMap[key] = value
	}
	return funcMap
}
