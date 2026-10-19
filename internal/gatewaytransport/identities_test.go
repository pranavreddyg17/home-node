package gatewaytransport

import "testing"

func TestPrivateRoleGroupsCannotAlias(t *testing.T) {
	valid := [4]uint32{1001, 1002, 1003, 1004}
	if err := ValidateRoleGroups(valid[0], valid[1], valid[2], valid[3]); err != nil {
		t.Fatal(err)
	}
	for first := range valid {
		for second := first + 1; second < len(valid); second++ {
			aliased := valid
			aliased[second] = aliased[first]
			if err := ValidateRoleGroups(aliased[0], aliased[1], aliased[2], aliased[3]); err == nil {
				t.Fatalf("role groups %d and %d shared permission authority", first, second)
			}
		}
		for _, invalid := range []uint32{0, 1 << 31} {
			groups := valid
			groups[first] = invalid
			if err := ValidateRoleGroups(groups[0], groups[1], groups[2], groups[3]); err == nil {
				t.Fatal("root or out-of-policy role group admitted")
			}
		}
	}
}
