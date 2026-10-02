package bench

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

// freeFormPrompt replaces systemPrompt for open tasks. It asks the model not
// to name the data format so the judge cannot tell the arms apart.
const freeFormPrompt = `You are a data assistant inside an application. Answer the user's request using only the tool result in the conversation. Be accurate and concise. Do not mention the format the data came in.`

func systemFor(q Question) string {
	if q.FreeForm {
		return freeFormPrompt
	}
	return ""
}

// FreeFormKinds are the open task templates, assigned to payloads in turn.
var FreeFormKinds = []string{"summary", "highlights", "describe", "compare"}

// FreeFormDatasets replaces each payload's Q&A questions with one open task
// with no gold answer. Payloads are shuffled with seed and take the kinds in
// FreeFormKinds in turn; ds and items are as PayloadDatasets returns them.
func FreeFormDatasets(ds []*Dataset, items []PayloadItem, seed uint64) ([]*Dataset, []PayloadItem) {
	r := rand.New(rand.NewPCG(seed, 0xf4ee))
	order := r.Perm(len(ds))
	var outD []*Dataset
	var outI []PayloadItem
	for n, i := range order {
		d, it := *ds[i], items[i]
		kind := FreeFormKinds[n%len(FreeFormKinds)]
		q, ok := freeFormTask(r, kind, it.Path, tableOf(d.Raw, it.Path))
		if !ok {
			kind = "summary"
			q, _ = freeFormTask(r, kind, it.Path, nil)
		}
		q.ID = strings.TrimPrefix(d.ID, "payload-")
		q.ID = "freeform-" + q.ID + "-" + kind
		d.Questions, it.Questions = []Question{q}, []Question{q}
		outD, outI = append(outD, &d), append(outI, it)
	}
	return outD, outI
}

// freeFormTask writes one open task. describe and compare need rows with an
// ID column; ok is false when there is none.
func freeFormTask(r *rand.Rand, kind, path string, rows []map[string]any) (Question, bool) {
	where := "the result"
	if path != "" {
		where = fmt.Sprintf("the %q list", path)
	}
	q := Question{Kind: "freeform-" + kind, FreeForm: true}
	switch kind {
	case "summary":
		q.Text = "Summarize this data for a colleague in 3 to 5 bullet points. Name specific items and include concrete values."
		return q, true
	case "highlights":
		q.Text = fmt.Sprintf("Which 3 items in %s stand out most? For each, explain in one or two sentences why, quoting its values.", where)
		return q, true
	}
	flat := make([]map[string]any, len(rows))
	var names []string
	seen := map[string]bool{}
	for i, row := range rows {
		flat[i] = map[string]any{}
		leaves("", row, flat[i])
		for c := range flat[i] {
			if !seen[c] {
				seen[c] = true
				names = append(names, c)
			}
		}
	}
	slices.Sort(names)
	id := idColumn(names, flat)
	if id == "" || len(flat) < 2 {
		return q, false
	}
	a := r.IntN(len(flat))
	if kind == "describe" {
		q.Text = fmt.Sprintf("Describe the item in %s whose %q is %s in a short paragraph, covering its most important fields and their values.",
			where, id, text(flat[a][id]))
		return q, true
	}
	b := (a + 1 + r.IntN(len(flat)-1)) % len(flat)
	q.Text = fmt.Sprintf("Compare the items in %s whose %q is %s and %s. List the fields where they differ, with both values.",
		where, id, text(flat[a][id]), text(flat[b][id]))
	return q, true
}

// judgePrompt is the judge's system prompt. The judge sees the data as
// compact JSON, whichever format the graded model was sent.
const judgePrompt = `You grade answers written by an AI data assistant. The tool result holds the data the assistant was given, as JSON. The user message holds the assistant's request and several answers labelled A, B, C. Grade each answer only against the data.

For each answer give:
- "errors": every statement in it that the data contradicts or does not support: wrong values, names or counts, invented items or fields. Quote each briefly. Rounding, paraphrase and ordering are not errors. An empty list means none.
- "complete": 1 to 5, how fully the answer does what the request asks (5 = fully).

Then for each pair of answers give "same": true if they state the same facts in substance (wording, order and layout aside) and do not conflict, else false.

Reply with JSON only, in exactly this shape:
{"A":{"errors":[],"complete":5},"B":{"errors":["..."],"complete":4},"C":{"errors":[],"complete":5},"same":{"AB":true,"AC":true,"BC":false}}`

// Grade is the judge's verdict on one answer.
type Grade struct {
	Errors   []string `json:"errors"`
	Complete int      `json:"complete"`
}

// JudgeRecord is one judge call: every arm's answer to one task by one model.
type JudgeRecord struct {
	Provider   string            `json:"provider"`
	Model      string            `json:"model"` // the graded model
	QuestionID string            `json:"question_id"`
	Kind       string            `json:"kind"`
	Judge      string            `json:"judge"`
	Labels     map[string]string `json:"labels"` // arm -> label shown to the judge
	Grades     map[string]Grade  `json:"grades"` // by arm
	Same       map[string]bool   `json:"same"`   // "armA|armB" (sorted) -> same facts
	Reply      string            `json:"reply"`
	// InputTokens and OutputTokens are the judge call's usage.
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	LatencyMS    int64  `json:"latency_ms"`
	Error        string `json:"error,omitempty"`
}

func (j JudgeRecord) key() string { return j.Provider + "|" + j.Model + "|" + j.QuestionID }

// JudgeConfig controls a judging run.
type JudgeConfig struct {
	Client      Client
	Model       string // the judge model
	Datasets    []*Dataset
	Records     []Record // free-form answers from Run
	Arms        []string // formats to grade together; every arm must have answered
	Out         string   // JSONL path; existing successful records are skipped
	Seed        uint64   // label shuffle
	MaxCalls    int
	Concurrency int
	RPM         int
	MaxTokens   int
}

// pairKey names an arm pair in sorted order.
func pairKey(a, b string) string {
	if b < a {
		a, b = b, a
	}
	return a + "|" + b
}

// Judge grades each (model, task) once, with the arms' answers under
// shuffled labels, against the data as JSON. A reply that is not valid JSON
// is retried up to twice more, then recorded as an error.
func Judge(ctx context.Context, cfg JudgeConfig) error {
	data := map[string]*Dataset{}
	for _, d := range cfg.Datasets {
		for _, q := range d.Questions {
			data[q.ID] = d
		}
	}
	done := map[string]bool{}
	if prev, err := ReadJudged(cfg.Out); err == nil {
		for _, j := range prev {
			if j.Error == "" {
				done[j.key()] = true
			}
		}
	}
	type job struct {
		g    *group
		qid  string
		arms map[string]Record
	}
	var jobs []job
	for _, g := range groupRecords(cfg.Records) {
		for _, qid := range g.qs {
			arms := g.byQ[qid]
			ok := data[qid] != nil
			for _, a := range cfg.Arms {
				_, has := arms[a]
				ok = ok && has
			}
			r := arms[cfg.Arms[0]]
			if ok && !done[(JudgeRecord{Provider: r.Provider, Model: r.Model, QuestionID: qid}).key()] {
				jobs = append(jobs, job{g, qid, arms})
			}
		}
	}
	log.Printf("judge: %d calls to make (%d already done)", len(jobs), len(done))
	if len(jobs) == 0 {
		return nil
	}
	if len(jobs) > cfg.MaxCalls {
		return fmt.Errorf("judge: %d calls planned, more than -max-calls %d; nothing was sent", len(jobs), cfg.MaxCalls)
	}
	f, err := os.OpenFile(cfg.Out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	tick := time.NewTicker(time.Minute / time.Duration(max(1, cfg.RPM)))
	defer tick.Stop()
	sem := make(chan struct{}, max(1, cfg.Concurrency))
	var mu sync.Mutex
	var wg sync.WaitGroup
	completed, failed := 0, 0
	for _, j := range jobs {
		select {
		case <-ctx.Done():
			wg.Wait()
			return ctx.Err()
		case <-tick.C:
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			defer func() { <-sem }()
			first := j.arms[cfg.Arms[0]]
			rec := JudgeRecord{Provider: first.Provider, Model: first.Model, QuestionID: j.qid, Kind: first.Kind, Judge: cfg.Model}
			labels, prompt := judgeInput(cfg.Arms, j.arms, first.Question, shuffleSeed(cfg.Seed, rec.key()))
			rec.Labels = labels
			d := data[j.qid]
			var err error
			for attempt := 0; attempt < 3; attempt++ {
				var res *Result
				cctx, cancel := context.WithTimeout(ctx, CallTimeout)
				res, err = cfg.Client.Do(cctx, Call{Model: cfg.Model, System: judgePrompt, Question: prompt,
					Tool: d.Tool, ToolArgs: d.ToolArgs, ToolResult: compactJSON(d), MaxTokens: cfg.MaxTokens})
				cancel()
				if err != nil {
					break
				}
				rec.Reply, rec.InputTokens, rec.OutputTokens, rec.LatencyMS = res.Text, res.InputTokens, res.OutputTokens, res.Latency.Milliseconds()
				if err = parseJudge(&rec, res.Text); err == nil {
					break
				}
			}
			if err != nil {
				rec.Error = err.Error()
			}
			line, _ := json.Marshal(rec)
			mu.Lock()
			defer mu.Unlock()
			w.Write(append(line, '\n'))
			w.Flush()
			completed++
			if err != nil {
				failed++
				log.Printf("judge: %s %s: %v", rec.Model, rec.QuestionID, err)
			}
			if completed%10 == 0 || completed == len(jobs) {
				log.Printf("judge: %d/%d done (%d errors)", completed, len(jobs), failed)
			}
		}(j)
	}
	wg.Wait()
	return nil
}

func shuffleSeed(seed uint64, key string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return seed ^ h.Sum64()
}

func compactJSON(d *Dataset) string {
	text, _, err := Render("json-compact", d)
	if err != nil {
		return string(d.JSON())
	}
	return text
}

// judgeInput labels the arms A, B, C… in a shuffled order and writes the
// judge's user message.
func judgeInput(arms []string, recs map[string]Record, request string, seed uint64) (map[string]string, string) {
	order := slices.Clone(arms)
	r := rand.New(rand.NewPCG(seed, 0x1abe1))
	r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	labels := map[string]string{}
	var b strings.Builder
	fmt.Fprintf(&b, "Request: %s\n", request)
	for i, a := range order {
		l := string(rune('A' + i))
		labels[a] = l
		ans := strings.TrimSpace(thinkRe.ReplaceAllString(recs[a].Reply, ""))
		if ans == "" {
			ans = "(no answer)"
		}
		fmt.Fprintf(&b, "\nAnswer %s:\n<<<\n%s\n>>>\n", l, ans)
	}
	return labels, b.String()
}

// parseJudge reads the judge's JSON into rec's grades by arm.
func parseJudge(rec *JudgeRecord, reply string) error {
	s := thinkRe.ReplaceAllString(reply, "")
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i < 0 || j < i {
		return fmt.Errorf("judge reply has no JSON object")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s[i:j+1]), &raw); err != nil {
		return fmt.Errorf("judge reply: %w", err)
	}
	byLabel := map[string]string{}
	for a, l := range rec.Labels {
		byLabel[l] = a
	}
	rec.Grades = map[string]Grade{}
	for l, a := range byLabel {
		var g Grade
		if err := json.Unmarshal(raw[l], &g); err != nil || g.Complete < 1 || g.Complete > 5 {
			return fmt.Errorf("judge reply: no valid grade for %s", l)
		}
		rec.Grades[a] = g
	}
	var same map[string]bool
	if err := json.Unmarshal(raw["same"], &same); err != nil {
		return fmt.Errorf("judge reply: no valid same: %v", err)
	}
	rec.Same = map[string]bool{}
	for l1, a1 := range byLabel {
		for l2, a2 := range byLabel {
			if l1 >= l2 {
				continue
			}
			v, ok := same[l1+l2]
			if !ok {
				return fmt.Errorf("judge reply: no same for %s%s", l1, l2)
			}
			rec.Same[pairKey(a1, a2)] = v
		}
	}
	return nil
}

// ReadJudged loads a judge JSONL file.
func ReadJudged(path string) ([]JudgeRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []JudgeRecord
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var j JudgeRecord
		if err := json.Unmarshal(sc.Bytes(), &j); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, sc.Err()
}

// wordF1 is the overlap of two replies' lowercased words (multiset F1).
func wordF1(a, b string) float64 {
	words := func(s string) map[string]int {
		m := map[string]int{}
		for _, w := range strings.FieldsFunc(strings.ToLower(thinkRe.ReplaceAllString(s, "")), func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '-'
		}) {
			m[strings.Trim(w, ".-")]++
		}
		delete(m, "")
		return m
	}
	wa, wb := words(a), words(b)
	na, nb, common := 0, 0, 0
	for w, n := range wa {
		na += n
		common += min(n, wb[w])
	}
	for _, n := range wb {
		nb += n
	}
	if na == 0 || nb == 0 {
		return 0
	}
	p, r := float64(common)/float64(nb), float64(common)/float64(na)
	if p+r == 0 {
		return 0
	}
	return 2 * p * r / (p + r)
}

// FreeFormMargin is the pre-registered bound: toonx may be described as
// showing no large drop on free-form answers only if the 95% upper bound of
// (toonx − json-compact) in the share of answers with an error is at most this.
const FreeFormMargin = 0.15

// ffItem is one judged task by one model.
type ffItem struct {
	j    JudgeRecord
	arms map[string]Record
}

// FreeFormReport writes the free-form check: per model and pooled, the
// share of answers with a judged error, completeness, agreement between arms
// (the judge's "same facts" and word overlap) against the json-compact-2
// noise floor, and tokens.
func FreeFormReport(w io.Writer, recs []Record, judged []JudgeRecord, cfg VerifyConfig) {
	byKey := map[string]map[string]Record{}
	calls, failed, empty := map[string]int{}, map[string]int{}, map[string]int{}
	for _, r := range recs {
		calls[r.Format]++
		if r.Error != "" {
			failed[r.Format]++
			continue
		}
		if r.Empty {
			empty[r.Format]++
		}
		k := r.Provider + "|" + r.Model + "|" + r.QuestionID
		if byKey[k] == nil {
			byKey[k] = map[string]Record{}
		}
		byKey[k][r.Format] = r
	}
	models := map[string][]ffItem{}
	var names []string
	jfail := 0
	for _, j := range judged {
		if j.Error != "" {
			jfail++
			continue
		}
		name := j.Provider + ":" + j.Model
		if models[name] == nil {
			names = append(names, name)
		}
		models[name] = append(models[name], ffItem{j, byKey[j.key()]})
	}
	sort.Strings(names)
	var all []ffItem
	for _, n := range names {
		all = append(all, models[n]...)
	}
	fmt.Fprintf(w, "Each task was answered with json-compact, json-compact-2 (the same JSON again: the noise floor) and toonx, then graded once by the judge with the answers under shuffled labels, against the data as JSON. Intervals are 95%% percentile bootstrap over tasks (%d resamples, seed %d).\n\n", cfg.Resamples, cfg.Seed)
	fmt.Fprintln(w, "## Quality by arm")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | tasks | answers with an error: json / json-2 / toonx | toonx − json | json-2 − json | complete (1–5): json / json-2 / toonx |")
	fmt.Fprintln(w, "|---|---:|---|---|---|---|")
	row := func(name string, items []ffItem) {
		errShare := func(arm string) func([]ffItem) float64 {
			return func(xs []ffItem) float64 {
				n := 0
				for _, x := range xs {
					if len(x.j.Grades[arm].Errors) > 0 {
						n++
					}
				}
				return float64(n) / float64(len(xs))
			}
		}
		diff := func(a, b string) func([]ffItem) float64 {
			return func(xs []ffItem) float64 { return errShare(a)(xs) - errShare(b)(xs) }
		}
		comp := func(arm string) float64 {
			s := 0
			for _, x := range items {
				s += x.j.Grades[arm].Complete
			}
			return float64(s) / float64(len(items))
		}
		dT := ffBootstrap(items, cfg, diff("toonx", "json-compact"))
		dN := ffBootstrap(items, cfg, diff("json-compact-2", "json-compact"))
		fmt.Fprintf(w, "| %s | %d | %.0f%% / %.0f%% / %.0f%% | %s | %s | %.2f / %.2f / %.2f |\n", name, len(items),
			100*errShare("json-compact")(items), 100*errShare("json-compact-2")(items), 100*errShare("toonx")(items),
			dT.pp(), dN.pp(), comp("json-compact"), comp("json-compact-2"), comp("toonx"))
	}
	for _, n := range names {
		row(n, models[n])
	}
	if len(names) > 1 {
		row("**all models**", all)
	}
	if len(all) > 0 {
		up := ffBootstrap(all, cfg, func(xs []ffItem) float64 {
			n := 0
			for _, x := range xs {
				if len(x.j.Grades["toonx"].Errors) > 0 {
					n++
				}
				if len(x.j.Grades["json-compact"].Errors) > 0 {
					n--
				}
			}
			return float64(n) / float64(len(xs))
		}).hi
		verdict := "no large drop shown"
		if up > FreeFormMargin {
			verdict = "a drop larger than the margin is not ruled out"
		}
		fmt.Fprintf(w, "\n**Pre-registered rule** (all models): upper bound of toonx − json %+.1fpp against a %.0fpp margin: %s.\n",
			100*up, 100*FreeFormMargin, verdict)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Do the answers agree?")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "\"Same facts\" is the judge's call on each pair. Word overlap is the F1 of the two replies' words (1 = same words). json vs json-2 is the model's own variation on identical input.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | same facts: json vs toonx | same facts: json vs json-2 | word overlap (median): json vs toonx | json vs json-2 |")
	fmt.Fprintln(w, "|---|---|---|---:|---:|")
	agree := func(name string, items []ffItem) {
		same := func(a, b string) func([]ffItem) float64 {
			return func(xs []ffItem) float64 {
				n := 0
				for _, x := range xs {
					if x.j.Same[pairKey(a, b)] {
						n++
					}
				}
				return float64(n) / float64(len(xs))
			}
		}
		f1 := func(a, b string) float64 {
			var xs []float64
			for _, x := range items {
				ra, okA := x.arms[a]
				rb, okB := x.arms[b]
				if okA && okB {
					xs = append(xs, wordF1(ra.Reply, rb.Reply))
				}
			}
			return median(xs)
		}
		fmt.Fprintf(w, "| %s | %s | %s | %.2f | %.2f |\n", name,
			ffBootstrap(items, cfg, same("json-compact", "toonx")).share(), ffBootstrap(items, cfg, same("json-compact", "json-compact-2")).share(),
			f1("json-compact", "toonx"), f1("json-compact", "json-compact-2"))
	}
	for _, n := range names {
		agree(n, models[n])
	}
	if len(names) > 1 {
		agree("**all models**", all)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Tokens")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| model | input saved by toonx | output tokens: json / toonx (mean) |")
	fmt.Fprintln(w, "|---|---:|---|")
	for _, n := range names {
		var inJ, inT, outJ, outT float64
		k := 0
		for _, x := range models[n] {
			rj, okJ := x.arms["json-compact"]
			rt, okT := x.arms["toonx"]
			if okJ && okT {
				inJ, inT = inJ+float64(rj.InputTokens), inT+float64(rt.InputTokens)
				outJ, outT = outJ+float64(rj.OutputTokens), outT+float64(rt.OutputTokens)
				k++
			}
		}
		if k == 0 {
			continue
		}
		fmt.Fprintf(w, "| %s | %.1f%% | %.0f / %.0f |\n", n, 100*(1-inT/inJ), outJ/float64(k), outT/float64(k))
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "## Calls")
	fmt.Fprintln(w)
	for _, fm := range slices.Sorted(func(yield func(string) bool) {
		for k := range calls {
			if !yield(k) {
				return
			}
		}
	}) {
		fmt.Fprintf(w, "- %s: %d calls, %d failed, %d empty replies (graded as given).\n", fm, calls[fm], failed[fm], empty[fm])
	}
	fmt.Fprintf(w, "- judge: %d records, %d not usable (excluded).\n", len(judged), jfail)
}

// ffBootstrap is a point estimate with a 95% percentile interval over tasks.
func ffBootstrap(items []ffItem, cfg VerifyConfig, f func([]ffItem) float64) interval {
	iv := interval{est: f(items)}
	if len(items) == 0 {
		return iv
	}
	r := rand.New(rand.NewPCG(cfg.Seed, 0xff))
	vals := make([]float64, cfg.Resamples)
	xs := make([]ffItem, len(items))
	for i := range vals {
		for k := range xs {
			xs[k] = items[r.IntN(len(items))]
		}
		vals[i] = f(xs)
	}
	sort.Float64s(vals)
	q := func(p float64) float64 { return vals[int(p*float64(len(vals)-1)+0.5)] }
	iv.lo, iv.hi = q(0.025), q(0.975)
	return iv
}

func (iv interval) share() string {
	return fmt.Sprintf("%.0f%% [%.0f, %.0f]", 100*iv.est, 100*iv.lo, 100*iv.hi)
}
