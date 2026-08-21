package hazard

import (
	"testing"

	"task141-firehydraulics/internal/model"
)

func TestParams(t *testing.T) {
	cases := []struct {
		class    model.HazardClass
		density  int64
		areaDM2  int64
	}{
		{model.HazardLight, 21, 13900},
		{model.HazardOrdinary1, 43, 13900},
		{model.HazardOrdinary2, 52, 13900},
		{model.HazardExtra1, 75, 23200},
		{model.HazardExtra2, 95, 23200},
		{model.HazardStorage, 80, 23200},
	}
	for _, c := range cases {
		p, ok := Params(c.class)
		if !ok {
			t.Errorf("Params(%s): not found", c.class)
			continue
		}
		if p.DensityMMPerMin != c.density {
			t.Errorf("Params(%s) density=%d want %d", c.class, p.DensityMMPerMin, c.density)
		}
		if p.DesignAreaDM2 != c.areaDM2 {
			t.Errorf("Params(%s) area=%d want %d", c.class, p.DesignAreaDM2, c.areaDM2)
		}
		if p.RemoteMinPressure != 500 {
			t.Errorf("Params(%s) remote min=%d want 500", c.class, p.RemoteMinPressure)
		}
	}
	if _, ok := Params("bogus"); ok {
		t.Errorf("unknown class should not be found")
	}
}

func TestValidate(t *testing.T) {
	// density below minimum → invalid
	if _, ok := Validate(model.HazardExtra2, 50, 23200, 500); ok {
		t.Errorf("density below minimum should be invalid")
	}
	// area mismatch → invalid
	if _, ok := Validate(model.HazardExtra2, 95, 13900, 500); ok {
		t.Errorf("area mismatch should be invalid")
	}
	// remote pressure below min → invalid
	if _, ok := Validate(model.HazardExtra2, 95, 23200, 400); ok {
		t.Errorf("remote pressure below min should be invalid")
	}
	// all consistent → valid
	if _, ok := Validate(model.HazardExtra2, 95, 23200, 500); !ok {
		t.Errorf("consistent params should be valid")
	}
	// density above minimum (override upward) → valid
	if _, ok := Validate(model.HazardExtra2, 120, 23200, 500); !ok {
		t.Errorf("density above minimum should be valid")
	}
}
