package domain

import "testing"

func TestViewerNeedsResolvedAccess(t *testing.T) {
	forged := Viewer{Role: "admin", Staff: StaffJury}
	if forged.IsAdmin() || forged.IsJury() || forged.IsStaff() || forged.CanPreview() {
		t.Fatal("role labels granted permissions without an access snapshot")
	}
	observer := Viewer{Access: &Access{Permissions: Permissions{ViewJury: true, PreviewProblems: true}}}
	if !observer.IsStaff() || !observer.CanPreview() || observer.IsJury() {
		t.Fatal("read-only staff permissions were conflated with jury mutation rights")
	}
}
