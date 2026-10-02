// Package bench implements the Phase 0 kill-or-go benchmark:
// synthetic but realistic tool-result tables with programmatically known
// answers, rendered in several formats and sent to real models.
package bench

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Question has a ground-truth answer computed from the data.
type Question struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // lookup | count | argmax | sum
	Text   string `json:"text"`
	Answer string `json:"answer"`
	// Numeric answers are compared as numbers; others as normalized strings.
	Numeric bool `json:"numeric"`
	// Answers lists every gold value when there are several (WTQ).
	Answers []string `json:"answers,omitempty"`
	// Check, when set, replaces the default scoring (Correct).
	Check func(reply string) bool `json:"-"`
	// FreeForm marks an open task with no gold answer: it is sent with
	// freeFormPrompt, never scored, and judged later (Judge).
	FreeForm bool `json:"free_form,omitempty"`
}

// Dataset is one tool result table plus its questions.
type Dataset struct {
	// ID, when set, identifies the table where Name, Rows and Seed do not
	// (one WTQ table per question).
	ID       string           `json:"id,omitempty"`
	Name     string           `json:"name"`
	Rows     int              `json:"rows"`
	Seed     uint64           `json:"seed"`
	Tool     string           `json:"tool"`
	ToolArgs string           `json:"tool_args"`
	Records  []map[string]any `json:"-"`
	// Raw, when set, is the tool result as the source has it (e.g. WTQ
	// columns in table order); Records is then unused.
	Raw       []byte     `json:"-"`
	Questions []Question `json:"questions"`
}

// Key identifies the dataset's content.
func (d *Dataset) Key() string {
	if d.ID != "" {
		return d.ID
	}
	return fmt.Sprintf("%s-%d-s%d", d.Name, d.Rows, d.Seed)
}

// JSON returns the tool result: Raw if set, else the records as a JSON
// array with keys in canonical order.
func (d *Dataset) JSON() []byte {
	if d.Raw != nil {
		return d.Raw
	}
	b, err := json.Marshal(d.Records) // map keys are sorted; numbers are json.Number
	if err != nil {
		panic(err)
	}
	return b
}

// Generators lists the dataset generators by name.
var Generators = map[string]func(r *rand.Rand, n int) *Dataset{
	"orders":    genOrders,
	"logs":      genLogs,
	"search":    genSearch,
	"employees": genEmployees,
}

// Generate builds every dataset at each size with a fixed seed.
func Generate(names []string, sizes []int, seed uint64) []*Dataset {
	var out []*Dataset
	for _, name := range names {
		for _, n := range sizes {
			r := rand.New(rand.NewPCG(seed, uint64(n)*1000+uint64(len(name))))
			d := Generators[name](r, n)
			d.Seed = seed
			if seed != PhaseZeroSeed {
				for i := range d.Questions {
					d.Questions[i].ID = fmt.Sprintf("%s-q%d", d.Key(), i+1)
				}
			}
			out = append(out, d)
		}
	}
	return out
}

// PhaseZeroSeed generated the Phase 0 datasets, whose question IDs
// ("orders-30-q1") predate the seed in the ID and are kept for its records.
const PhaseZeroSeed = 42

// GenerateSeeds is Generate for seeds seed..seed+n-1.
func GenerateSeeds(names []string, sizes []int, seed uint64, n int) []*Dataset {
	var out []*Dataset
	for i := 0; i < max(n, 1); i++ {
		out = append(out, Generate(names, sizes, seed+uint64(i))...)
	}
	return out
}

func num(s string) json.Number { return json.Number(s) }
func numI(i int) json.Number   { return json.Number(strconv.Itoa(i)) }

// money returns a 2-decimal amount in shortest float form (as JS/Python
// serializers emit it), e.g. 1234.5 rather than 1234.50.
func money(cents int) json.Number {
	return json.Number(strconv.FormatFloat(float64(cents)/100, 'f', -1, 64))
}

func pick[T any](r *rand.Rand, xs []T) T { return xs[r.IntN(len(xs))] }

// distinct returns k distinct indices in [0,n).
func distinct(r *rand.Rand, n, k int) []int {
	return r.Perm(n)[:min(k, n)]
}

var (
	companyA = []string{"Acme", "Globex", "Initech", "Umbrella", "Stark", "Wayne", "Hooli", "Vandelay", "Soylent", "Tyrell", "Cyberdyne", "Wonka", "Gringotts", "Monarch", "Aperture", "Oscorp"}
	companyB = []string{"Corp", "Industries", "Labs", "Systems", "Holdings", "Logistics", "Foods", "Partners"}
	firsts   = []string{"Ana", "Bilal", "Chen", "Daria", "Emeka", "Farah", "Goran", "Hana", "Ivan", "Jia", "Kofi", "Lena", "Mateo", "Nora", "Omar", "Priya", "Quinn", "Rosa", "Sven", "Tara", "Uma", "Viktor", "Wen", "Yusuf", "Zoe"}
	lasts    = []string{"Okafor", "Lindqvist", "Tanaka", "Moreau", "Haddad", "Kowalski", "Nguyen", "Fischer", "Silva", "Petrov", "Rossi", "Iyer", "Brennan", "Castillo", "Novak", "Sato", "Mbeki", "Larsen", "Duarte", "Weiss"}
	words    = strings.Fields("latency cache shard replica index vector query token budget router gateway schema policy audit tenant region cluster deploy rollout canary metric trace span queue worker batch stream retry backoff quota limit lease leader follower snapshot compaction partition")
)

func genOrders(r *rand.Rand, n int) *Dataset {
	statuses := []string{"paid", "pending", "refunded", "shipped"}
	currencies := []string{"USD", "EUR", "GBP"}
	nCust := max(4, n/4)
	customers := make([]string, nCust)
	seen := map[string]bool{}
	for i := range customers {
		for {
			c := pick(r, companyA) + " " + pick(r, companyB)
			if !seen[c] {
				seen[c], customers[i] = true, c
				break
			}
		}
	}
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	recs := make([]map[string]any, n)
	cents := make([]int, n)
	for i := range recs {
		cents[i] = 500 + r.IntN(250000)
		recs[i] = map[string]any{
			"id":         numI(10001 + i),
			"customer":   pick(r, customers),
			"status":     pick(r, statuses),
			"amount":     money(cents[i]),
			"currency":   pick(r, currencies),
			"items":      numI(1 + r.IntN(12)),
			"created_at": base.Add(time.Duration(r.IntN(90*24*60)) * time.Minute).Format(time.RFC3339),
		}
	}
	d := &Dataset{Name: "orders", Rows: n, Tool: "query_orders", ToolArgs: `{"since":"2026-03-01","limit":` + strconv.Itoa(n) + `}`, Records: recs}
	idx := distinct(r, n, 3)
	d.add("lookup", fmt.Sprintf("What is the customer name on order id %s?", recs[idx[0]]["id"]), recs[idx[0]]["customer"].(string), false)
	d.add("lookup", fmt.Sprintf("What is the amount of order id %s?", recs[idx[1]]["id"]), string(recs[idx[1]]["amount"].(json.Number)), true)
	st := pick(r, statuses)
	d.add("count", fmt.Sprintf("How many orders have status %q?", st), strconv.Itoa(countWhere(recs, func(m map[string]any) bool { return m["status"] == st })), true)
	cur := pick(r, currencies)
	d.add("count", fmt.Sprintf("How many orders have status \"paid\" and currency %q?", cur), strconv.Itoa(countWhere(recs, func(m map[string]any) bool { return m["status"] == "paid" && m["currency"] == cur })), true)
	best := 0
	for i := range cents {
		if cents[i] > cents[best] {
			best = i
		}
	}
	d.add("argmax", "Which order id has the largest amount?", string(recs[best]["id"].(json.Number)), true)
	// Sum of items for a customer with few orders, to keep arithmetic small.
	byCust := map[string][]int{}
	for i, m := range recs {
		byCust[m["customer"].(string)] = append(byCust[m["customer"].(string)], i)
	}
	cands := []string{}
	for c, is := range byCust {
		if len(is) >= 2 && len(is) <= 5 {
			cands = append(cands, c)
		}
	}
	slices.Sort(cands)
	if len(cands) > 0 {
		c := pick(r, cands)
		sum := 0
		for _, i := range byCust[c] {
			v, _ := strconv.Atoi(string(recs[i]["items"].(json.Number)))
			sum += v
		}
		d.add("sum", fmt.Sprintf("What is the total number of items across all orders from customer %q?", c), strconv.Itoa(sum), true)
	}
	d.add("lookup", fmt.Sprintf("What is the created_at timestamp of order id %s?", recs[idx[2]]["id"]), recs[idx[2]]["created_at"].(string), false)
	return d
}

func genLogs(r *rand.Rand, n int) *Dataset {
	levels := []string{"INFO", "INFO", "INFO", "WARN", "ERROR", "DEBUG"}
	services := []string{"auth", "billing", "search", "api", "worker"}
	codes := []int{200, 200, 200, 201, 204, 400, 404, 429, 500, 503}
	msgs := []string{"request completed", "upstream timeout", "cache miss", "token refreshed", "rate limited by upstream", "retrying request", "connection reset by peer", "payment authorized", "index rebuilt", "invalid signature"}
	base := time.Date(2026, 9, 30, 14, 0, 0, 0, time.UTC)
	recs := make([]map[string]any, n)
	lat := make([]int, n)
	for i := range recs {
		lat[i] = 3 + r.IntN(4000)
		recs[i] = map[string]any{
			"ts":         base.Add(time.Duration(i*1700+r.IntN(1500)) * time.Millisecond).Format("2006-01-02T15:04:05.000Z"),
			"level":      pick(r, levels),
			"service":    pick(r, services),
			"latency_ms": numI(lat[i]),
			"status":     numI(pick(r, codes)),
			"trace_id":   fmt.Sprintf("%016x", r.Uint64()),
			"message":    pick(r, msgs),
		}
	}
	d := &Dataset{Name: "logs", Rows: n, Tool: "search_logs", ToolArgs: `{"window":"1h","limit":` + strconv.Itoa(n) + `}`, Records: recs}
	idx := distinct(r, n, 3)
	d.add("lookup", fmt.Sprintf("What is the latency_ms of the log entry with trace_id %s?", recs[idx[0]]["trace_id"]), string(recs[idx[0]]["latency_ms"].(json.Number)), true)
	svc := pick(r, services)
	d.add("count", fmt.Sprintf("How many ERROR-level entries are from service %q?", svc), strconv.Itoa(countWhere(recs, func(m map[string]any) bool { return m["level"] == "ERROR" && m["service"] == svc })), true)
	best := 0
	for i := range lat {
		if lat[i] > lat[best] {
			best = i
		}
	}
	d.add("argmax", "Which trace_id has the highest latency_ms?", recs[best]["trace_id"].(string), false)
	d.add("count", "How many entries have status 500?", strconv.Itoa(countWhere(recs, func(m map[string]any) bool { return m["status"] == numI(500) })), true)
	d.add("lookup", fmt.Sprintf("Which service logged the entry at ts %s?", recs[idx[1]]["ts"]), recs[idx[1]]["service"].(string), false)
	d.add("lookup", fmt.Sprintf("What is the message of the entry with trace_id %s?", recs[idx[2]]["trace_id"]), recs[idx[2]]["message"].(string), false)
	return d
}

func genSearch(r *rand.Rand, n int) *Dataset {
	domains := []string{"docs.example.com", "blog.example.org", "wiki.internal", "support.example.com", "kb.example.net"}
	recs := make([]map[string]any, n)
	scores := make([]float64, n)
	for i := range scores {
		scores[i] = float64(r.IntN(9000)+1000) / 10000
	}
	slices.SortFunc(scores, func(a, b float64) int { return cmp.Compare(b, a) })
	titles := map[string]bool{}
	for i := range recs {
		var title string
		for {
			title = strings.ToUpper(pick(r, words)[:1]) + strings.Join([]string{pick(r, words)[1:], pick(r, words), pick(r, words), "guide"}, " ")
			if !titles[title] {
				titles[title] = true
				break
			}
		}
		snip := make([]string, 22+r.IntN(10))
		for j := range snip {
			snip[j] = pick(r, words)
		}
		slug := strings.ReplaceAll(strings.ToLower(title), " ", "-")
		recs[i] = map[string]any{
			"rank":    numI(i + 1),
			"doc_id":  fmt.Sprintf("D-%05d", r.IntN(100000)),
			"title":   title,
			"url":     "https://" + pick(r, domains) + "/" + slug,
			"score":   num(strconv.FormatFloat(scores[i], 'f', -1, 64)),
			"snippet": strings.Join(snip, " ") + ".",
		}
	}
	d := &Dataset{Name: "search", Rows: n, Tool: "vector_search", ToolArgs: `{"query":"gateway retry policy","k":` + strconv.Itoa(n) + `}`, Records: recs}
	idx := distinct(r, n, 3)
	d.add("lookup", fmt.Sprintf("What is the url of the result titled %q?", recs[idx[0]]["title"]), recs[idx[0]]["url"].(string), false)
	d.add("lookup", fmt.Sprintf("What is the doc_id of the result with rank %d?", idx[1]+1), recs[idx[1]]["doc_id"].(string), false)
	d.add("lookup", fmt.Sprintf("What is the score of document %s?", recs[idx[2]]["doc_id"]), string(recs[idx[2]]["score"].(json.Number)), true)
	dom := pick(r, domains)
	d.add("count", fmt.Sprintf("How many results have a url on the domain %s?", dom), strconv.Itoa(countWhere(recs, func(m map[string]any) bool { return strings.HasPrefix(m["url"].(string), "https://"+dom+"/") })), true)
	return d
}

func genEmployees(r *rand.Rand, n int) *Dataset {
	depts := []string{"Engineering", "Sales", "Finance", "Support", "Marketing"}
	titles := map[string][]string{"Engineering": {"Engineer", "Senior Engineer", "Staff Engineer"}, "Sales": {"Account Executive", "Sales Manager"}, "Finance": {"Analyst", "Controller"}, "Support": {"Support Specialist", "Support Lead"}, "Marketing": {"Marketing Manager", "Content Strategist"}}
	locs := []string{"Berlin", "Austin", "Bangalore", "Toronto", "Lisbon"}
	levels := []string{"L1", "L2", "L3", "L4", "L5"}
	recs := make([]map[string]any, n)
	names := map[string]bool{}
	base := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range recs {
		var f, l string
		for {
			f, l = pick(r, firsts), pick(r, lasts)
			if !names[f+l] {
				names[f+l] = true
				break
			}
		}
		dept := pick(r, depts)
		mgr := any(nil)
		if i > 0 {
			mgr = numI(5001 + r.IntN(i))
		}
		recs[i] = map[string]any{
			"id":         numI(5001 + i),
			"first_name": f,
			"last_name":  l,
			"email":      strings.ToLower(f+"."+l) + "@example.com",
			"department": dept,
			"title":      pick(r, titles[dept]),
			"level":      pick(r, levels),
			"salary":     numI(45000 + 1000*r.IntN(150)),
			"start_date": base.AddDate(0, 0, r.IntN(4000)).Format("2006-01-02"),
			"location":   pick(r, locs),
			"manager_id": mgr,
			"active":     r.IntN(10) > 0,
		}
	}
	d := &Dataset{Name: "employees", Rows: n, Tool: "list_employees", ToolArgs: `{"include_inactive":true}`, Records: recs}
	idx := distinct(r, n, 3)
	d.add("lookup", fmt.Sprintf("What is the email of employee id %s?", recs[idx[0]]["id"]), recs[idx[0]]["email"].(string), false)
	dept, loc := pick(r, depts), pick(r, locs)
	d.add("count", fmt.Sprintf("How many active employees are in department %q and located in %s?", dept, loc), strconv.Itoa(countWhere(recs, func(m map[string]any) bool {
		return m["active"] == true && m["department"] == dept && m["location"] == loc
	})), true)
	sd := pick(r, depts)
	best := -1
	for i, m := range recs {
		if m["department"] != sd {
			continue
		}
		if best < 0 || salary(m) > salary(recs[best]) {
			best = i
		}
	}
	if best >= 0 && countWhere(recs, func(m map[string]any) bool { return m["department"] == sd && salary(m) == salary(recs[best]) }) == 1 {
		d.add("argmax", fmt.Sprintf("Which employee in department %q has the highest salary? Answer with their first and last name.", sd), recs[best]["first_name"].(string)+" "+recs[best]["last_name"].(string), false)
	}
	j := max(1, idx[1])
	d.add("lookup", fmt.Sprintf("What is the manager_id of %s %s?", recs[j]["first_name"], recs[j]["last_name"]), string(recs[j]["manager_id"].(json.Number)), true)
	lv := pick(r, levels)
	d.add("count", fmt.Sprintf("How many employees have level %s?", lv), strconv.Itoa(countWhere(recs, func(m map[string]any) bool { return m["level"] == lv })), true)
	d.add("lookup", fmt.Sprintf("What is the start_date of employee id %s?", recs[idx[2]]["id"]), recs[idx[2]]["start_date"].(string), false)
	return d
}

func salary(m map[string]any) int {
	v, _ := strconv.Atoi(string(m["salary"].(json.Number)))
	return v
}

func countWhere(recs []map[string]any, f func(map[string]any) bool) int {
	c := 0
	for _, m := range recs {
		if f(m) {
			c++
		}
	}
	return c
}

func (d *Dataset) add(kind, text, answer string, numeric bool) {
	d.Questions = append(d.Questions, Question{
		ID:      fmt.Sprintf("%s-%d-q%d", d.Name, d.Rows, len(d.Questions)+1),
		Kind:    kind,
		Text:    text,
		Answer:  answer,
		Numeric: numeric,
	})
}
