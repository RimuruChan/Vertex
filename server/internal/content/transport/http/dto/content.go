package dto

import contentdomain "github.com/RimuruChan/Vertex/server/internal/content/domain"

type ContentPermissions struct {
	View     bool `json:"view"`
	ViewBody bool `json:"viewBody"`
	Edit     bool `json:"edit"`
	Delete   bool `json:"delete"`
	Comment  bool `json:"comment"`
	Vote     bool `json:"vote"`
}

func FromPermissions(p contentdomain.Permissions) ContentPermissions {
	return ContentPermissions{View: p.View, ViewBody: p.ViewBody, Edit: p.Edit, Delete: p.Delete, Comment: p.Comment, Vote: p.Vote}
}
