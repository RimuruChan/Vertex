package domain

import "regexp"

var languagePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
var judgeVerdicts = map[string]bool{
	"Accepted": true, "Wrong Answer": true, "Time Limit Exceeded": true,
	"Memory Limit Exceeded": true, "Runtime Error": true,
	"Presentation Error": true, "Any Rejection": true,
}
