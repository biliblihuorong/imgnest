package service

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordCostIsProductionStrengthOutsideTestBinaries(t *testing.T) {
	if got := passwordCostFor(false); got != 12 {
		t.Fatalf("production bcrypt cost=%d; want 12", got)
	}
	if got := passwordCostFor(true); got != bcrypt.MinCost {
		t.Fatalf("test binary bcrypt cost=%d; want %d", got, bcrypt.MinCost)
	}
	if passwordCost != bcrypt.MinCost {
		t.Fatalf("cost in effect under go test=%d; want %d", passwordCost, bcrypt.MinCost)
	}
}

func TestUseProductionPasswordCostRestoresTheTestCost(t *testing.T) {
	t.Run("raised", func(t *testing.T) {
		UseProductionPasswordCost(t)
		if passwordCost != productionPasswordCost {
			t.Fatalf("cost=%d; want %d", passwordCost, productionPasswordCost)
		}
	})
	if passwordCost != bcrypt.MinCost {
		t.Fatalf("cost after cleanup=%d; want %d", passwordCost, bcrypt.MinCost)
	}
}
