package model

import (
	"reflect"
	"testing"
	"time"
)

func TestGroupByCWDCaseOrder(t *testing.T) {
	sessions := []Session{
		{CWD: "/a", LastActivity: time.Now()},
		{CWD: "/b", LastActivity: time.Now()},
		{CWD: "", LastActivity: time.Now()},
		{CWD: "/b", LastActivity: time.Now()},
		{CWD: "/a", LastActivity: time.Now()},
		{CWD: "", LastActivity: time.Now()},
	}
	groups := GroupByCWD(sessions)

	want := []string{"/a", "/b", ""}
	got := make([]string, 0, len(groups))
	for _, g := range groups {
		got = append(got, g.Path)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group order = %v, want %v", got, want)
	}
	if groups[2].GroupCount() != 2 {
		t.Fatalf("anonymous bucket count = %d, want 2", groups[2].GroupCount())
	}
}

func TestAgeMatchesStrictlyOlder(t *testing.T) {
	now := time.Now()
	age := Ages[1] // 3d

	if !age.Matches(now.Add(-4 * 24 * time.Hour)) {
		t.Fatal("session 4 days old should match 3d bucket")
	}
	if age.Matches(now.Add(-3*24*time.Hour + time.Minute)) {
		t.Fatal("session just inside the 3d boundary must not match strictly-older 3d bucket")
	}
	if !Ages[6].Matches(now) { // all
		t.Fatal("all bucket should match anything")
	}
}
