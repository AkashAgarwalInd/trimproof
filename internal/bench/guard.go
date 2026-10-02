package bench

import (
	"fmt"
	"net/url"
	"strings"
)

// FreeHosts are the endpoints live benchmark calls may reach without
// -allow-paid: free tiers that cannot bill (NVIDIA's developer API, GitHub
// Models, the Gemini API's free tier) and a gateway on this machine.
var FreeHosts = []string{
	"integrate.api.nvidia.com",
	"models.github.ai",
	"generativelanguage.googleapis.com",
	"localhost",
	"127.0.0.1",
	"::1",
}

// CheckEndpoint refuses a base URL outside FreeHosts unless allowPaid is
// set, so a benchmark cannot spend money by accident (for example by
// falling back to api.openai.com with whatever key is in the environment).
func CheckEndpoint(base string, allowPaid bool) error {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return fmt.Errorf("bad endpoint %q", base)
	}
	h := strings.ToLower(u.Hostname())
	for _, f := range FreeHosts {
		if h == f {
			return nil
		}
	}
	if allowPaid {
		return nil
	}
	return fmt.Errorf("%s is not a free endpoint (%s); pass -allow-paid to call it anyway",
		h, strings.Join(FreeHosts, ", "))
}
