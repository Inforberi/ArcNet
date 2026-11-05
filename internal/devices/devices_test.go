package devices

import "testing"

func TestCredentialPolicyBoundsAndUniqueness(t *testing.T) {
	ids := map[int64]string{1: "11111111-1111-4111-8111-111111111111"}
	d := Device{ID: "22222222-2222-4222-8222-222222222222", UUID: "33333333-3333-4333-8333-333333333333", Token: "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", Name: "Телефон", CreatedAt: 100}
	r := Record{UserID: 1, Identity: ids[1], Limit: 1, Version: 1, Devices: []Device{d}}
	if err := (State{r}).Validate(ids); err != nil {
		t.Fatal(err)
	}
	r.Devices = append(r.Devices, d)
	if (State{r}).Validate(ids) == nil {
		t.Fatal("duplicate or over-limit accepted")
	}
	r.Devices = r.Devices[:1]
	r.Devices[0].UUID = ids[1]
	if (State{r}).Validate(ids) == nil {
		t.Fatal("legacy credential reused")
	}
	for _, name := range []string{"", " x", "x\ny"} {
		if ValidName(name) {
			t.Fatal("invalid name", name)
		}
	}
}
