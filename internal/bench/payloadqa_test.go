package bench

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/AkashAgarwalInd/trimproof/pkg/canonical"
	"github.com/AkashAgarwalInd/trimproof/pkg/tokens"
)

func TestPayloadQuestions(t *testing.T) {
	raw := `{"total":6,"items":[
		{"id":1,"state":"open","user":{"login":"ann"},"stars":5,"draft":true},
		{"id":2,"state":"closed","user":{"login":"bo"},"stars":9},
		{"id":3,"state":"open","user":{"login":"cy"},"stars":7,"draft":false},
		{"id":4,"state":"open","user":{"login":"di"},"stars":1},
		{"id":5,"state":"closed","user":{"login":"ed"},"stars":2,"draft":null},
		{"id":6,"state":"open","user":{"login":"fa"},"stars":3}]}`
	it, body, ok := payloadItem([]byte(raw), tokens.NewCalibrated(nil))
	if !ok || it.Path != "items" || it.KeptRows != 6 {
		t.Fatalf("payloadItem: %+v %v", it, ok)
	}
	if want, _ := canonical.Canonicalize([]byte(raw)); string(body) != string(want) {
		t.Fatalf("body is not the canonical payload:\n%s", body)
	}
	rows := tableOf(body, it.Path)
	qs := payloadQuestions(rand.New(rand.NewPCG(1, 1)), it.Path, rows)
	kinds := map[string]Question{}
	for _, q := range qs {
		kinds[q.Kind] = q
	}
	if len(qs) != payloadMaxQuestions {
		t.Fatalf("%d questions: %+v", len(qs), qs)
	}
	if q := kinds["lookup-nested"]; !strings.Contains(q.Text, `"user.login"`) || q.Answer == "" {
		t.Errorf("nested lookup: %+v", q)
	}
	if q := kinds["lookup-sparse"]; !strings.Contains(q.Text, `"draft"`) {
		t.Errorf("sparse lookup: %+v", q)
	} else if strings.Contains(q.Text, `is 1?`) && q.Answer != "true" || strings.Contains(q.Text, `is 2?`) && q.Answer != "none" {
		t.Errorf("sparse lookup gold: %+v", q)
	}
	if q := kinds["argmax"]; q.Answer != "2" || !strings.Contains(q.Text, `"stars"`) {
		t.Errorf("argmax: %+v", q)
	}
	if q, ok := kinds["count-where"]; ok {
		if strings.Contains(q.Text, "equal to open") && q.Answer != "4" || strings.Contains(q.Text, "equal to closed") && q.Answer != "2" {
			t.Errorf("count-where: %+v", q)
		}
	}
	// A payload without a table of at least payloadMinRows rows is skipped.
	if _, _, ok := payloadItem([]byte(`{"items":[{"id":1},{"id":2}]}`), tokens.NewCalibrated(nil)); ok {
		t.Fatal("short table accepted")
	}
}
