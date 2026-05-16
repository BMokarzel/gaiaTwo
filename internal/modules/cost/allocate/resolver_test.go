package allocate

import (
	"context"
	"errors"
	"testing"
	"time"

	"costEngine/internal/entity/node"
	"costEngine/internal/modules/bridge"
	"costEngine/internal/repository"
)

// fakeResolver implementa bridge.Resolver com mapas estáticos.
type fakeResolver struct {
	// hits: (account, externalID) → URN
	hits map[[2]string]node.URN
	// ambig: (account, externalID) → true → retorna ErrAmbiguous
	ambig map[[2]string]bool
	// fatal: se != nil, retorna esse erro como erro fatal (não-sentinela)
	fatal error
}

func (f *fakeResolver) ResolveURN(ctx context.Context, p node.ProviderID, account, externalID string, as repository.AsOf) (node.URN, error) {
	if f.fatal != nil {
		return "", f.fatal
	}
	if f.ambig[[2]string{account, externalID}] {
		return "", repository.ErrAmbiguous
	}
	urn, ok := f.hits[[2]string{account, externalID}]
	if !ok {
		return "", repository.ErrNotFound
	}
	return urn, nil
}

func (f *fakeResolver) ResolveURNBatch(ctx context.Context, p node.ProviderID, account string, ids []string, as repository.AsOf) (map[string]node.URN, []bridge.ResolveError, error) {
	if f.fatal != nil {
		return nil, nil, f.fatal
	}
	out := map[string]node.URN{}
	var failures []bridge.ResolveError
	for _, id := range ids {
		urn, err := f.ResolveURN(ctx, p, account, id, as)
		if err != nil {
			failures = append(failures, bridge.ResolveError{ExternalID: id, Err: err})
			continue
		}
		out[id] = urn
	}
	return out, failures, nil
}

func TestAsOfForPeriod(t *testing.T) {
	// Maio 2026: AsOf deve ser 2026-05-31 23:59:59.999 UTC.
	may1 := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	asOf := AsOfForPeriod(may1).Time()
	want := time.Date(2026, 5, 31, 23, 59, 59, int(999*time.Millisecond), time.UTC)
	if !asOf.Equal(want) {
		t.Errorf("AsOf = %v, want %v", asOf, want)
	}
}

func TestResolve_HitsMissAmbig(t *testing.T) {
	r := &fakeResolver{
		hits: map[[2]string]node.URN{
			{"111", "i-1"}: "urn:ce:aws:111:compute/i-1",
			{"111", "i-2"}: "urn:ce:aws:111:compute/i-2",
			{"222", "i-1"}: "urn:ce:aws:222:compute/i-1",
		},
		ambig: map[[2]string]bool{
			{"111", "shared-bucket"}: true,
		},
	}
	ids := map[string][]string{
		"111": {"i-1", "i-2", "shared-bucket", "missing"},
		"222": {"i-1"},
	}
	res, err := Resolve(context.Background(), r, node.ProviderAWS, ids,
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got := res.URNByResource[resourceLookup{"111", "i-1"}]; got != "urn:ce:aws:111:compute/i-1" {
		t.Errorf("(111,i-1) URN=%q", got)
	}
	if got := res.URNByResource[resourceLookup{"222", "i-1"}]; got != "urn:ce:aws:222:compute/i-1" {
		t.Errorf("(222,i-1) URN=%q", got)
	}
	if res.Failures[resourceLookup{"111", "shared-bucket"}] != ReasonAmbiguous {
		t.Errorf("shared-bucket should be Ambiguous, got %v", res.Failures[resourceLookup{"111", "shared-bucket"}])
	}
	if res.Failures[resourceLookup{"111", "missing"}] != ReasonNotFound {
		t.Errorf("missing should be NotFound")
	}
}

func TestResolve_FatalAborts(t *testing.T) {
	boom := errors.New("boom")
	r := &fakeResolver{fatal: boom}
	_, err := Resolve(context.Background(), r, node.ProviderAWS,
		map[string][]string{"111": {"i-1"}},
		time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, boom) {
		t.Errorf("err=%v, want wrap of boom", err)
	}
}
