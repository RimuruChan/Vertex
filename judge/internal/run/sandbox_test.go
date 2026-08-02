package run

import "testing"

func TestValidateBoxName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		want bool
	}{
		{"", false},
		{"input.txt", true},
		{"dir/input.txt", false},
		{`dir\input.txt`, false},
		{".", false},
		{"../secret", false},
		{"dir/../../secret", false},
		{"/etc/passwd", false},
		{"dir/../input.txt", false},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := validateBoxName(test.name) == nil; got != test.want {
				t.Fatalf("validateBoxName(%q) valid = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestRunnerEnvironmentIsAllowlisted(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://secret")
	t.Setenv("VERTEX_CGROUP_ROOT", "/sys/fs/cgroup/test")
	environment := runnerEnvironment()
	for _, entry := range environment {
		if entry == "DATABASE_URL=postgres://secret" {
			t.Fatal("runner environment leaked DATABASE_URL")
		}
	}
}
