package evaluation

import "testing"

func TestStoresFrom_Nil(t *testing.T) {
	s := StoresFrom(nil)
	if s.Datasets != nil || s.Cases != nil || s.Runs != nil || s.RunQueries != nil || s.Results != nil || s.Governance != nil {
		t.Fatal("StoresFrom(nil) must yield a zero Stores")
	}
}

func TestNewUsecase_BindsStores(t *testing.T) {
	repo := &mockRepo{}
	uc := NewUsecase(StoresFrom(repo), nil)
	if uc.datasets != repo || uc.cases != repo || uc.runs != repo || uc.runQueries != repo || uc.results != repo || uc.gov != repo {
		t.Fatal("NewUsecase must bind each Stores field to the same adapter")
	}
}
