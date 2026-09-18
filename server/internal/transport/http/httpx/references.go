package httpx

import (
	"context"
	"errors"
	"net/http"
	"reflect"

	reference "github.com/RimuruChan/Vertex/server/internal/shared/resourceid"
	"github.com/gin-gonic/gin"
)

type NumberResolver interface {
	Resolve(context.Context, string, string) (string, error)
}

const referenceResolver = "vertex.numberResolver"

// BindReferences only supplies the dependency. Resource routers explicitly
// declare their parameters; no URL parsing or request mutation occurs here.
func BindReferences(resolver NumberResolver) gin.HandlerFunc {
	return func(c *gin.Context) { c.Set(referenceResolver, resolver); c.Next() }
}

func ResolveReference(c *gin.Context, kind, number string) (string, bool) {
	value, err := reference.ParseNumber(number)
	if err != nil {
		referenceError(c, reference.ErrNotFound)
		return "", false
	}
	resolver, _ := c.Get(referenceResolver)
	lookup, ok := resolver.(NumberResolver)
	if !ok || lookup == nil {
		referenceError(c, errors.New("number resolver is not configured"))
		return "", false
	}
	id, err := lookup.Resolve(c.Request.Context(), kind, value.String())
	if err != nil {
		referenceError(c, err)
		return "", false
	}
	return id, true
}

func NumberParam(kind, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if value := c.Param(key); value != "" {
			id, ok := ResolveReference(c, kind, value)
			if !ok {
				return
			}
			c.Set("vertex.param."+key, id)
		}
		c.Next()
	}
}

// RequireNumberParam is for tenant governance, which resolves its own reference.
func RequireNumberParam(key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := reference.ParseNumber(c.Param(key)); err != nil {
			referenceError(c, err)
			return
		}
		c.Next()
	}
}
func NumberQuery(kind, key string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if value := c.Query(key); value != "" {
			id, ok := ResolveReference(c, kind, value)
			if !ok {
				return
			}
			c.Set("vertex.query."+key, id)
		}
		c.Next()
	}
}
func ResourceID(c *gin.Context, key string) string {
	if value, ok := c.Get("vertex.param." + key); ok {
		return value.(string)
	}
	return c.Param(key)
}
func ResourceQuery(c *gin.Context, key string) string {
	if value, ok := c.Get("vertex.query." + key); ok {
		return value.(string)
	}
	return c.Query(key)
}

// ResolveBodyReferences follows only explicit DTO resource tags. Actor IDs,
// task IDs and untagged strings are never treated as resource numbers.
func ResolveBodyReferences(c *gin.Context, request any) bool {
	var walk func(reflect.Value, string) bool
	walk = func(v reflect.Value, kind string) bool {
		if v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return true
			}
			return walk(v.Elem(), kind)
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).PkgPath != "" {
					continue
				}
				if !walk(v.Field(i), v.Type().Field(i).Tag.Get("resource")) {
					return false
				}
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				if !walk(v.Index(i), kind) {
					return false
				}
			}
		case reflect.String:
			if kind != "" && v.String() != "" && v.CanSet() {
				id, ok := ResolveReference(c, kind, v.String())
				if !ok {
					return false
				}
				v.SetString(id)
			}
		}
		return true
	}
	return walk(reflect.ValueOf(request), "")
}
func referenceError(c *gin.Context, err error) {
	if errors.Is(err, reference.ErrNotFound) {
		WriteError(c, http.StatusNotFound, "resource.not_found", "资源不存在")
	} else {
		_ = c.Error(err)
		WriteError(c, 500, "internal.error", "服务器内部错误")
	}
	c.Abort()
}
