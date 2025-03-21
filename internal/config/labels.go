package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/ameistad/turkis/internal/helpers"
	"github.com/fatih/color"
)

const (
	LabelAppName         = "turkis.appName"
	LabelDeploymentID    = "turkis.deployment-id"
	LabelIgnore          = "turkis.ignore"            // optional
	LabelHealthCheckPath = "turkis.health-check-path" // optional default to "/"
	LabelACMEEmail       = "turkis.acme.email"
	LabelPort            = "turkis.port" // optional

	// Format strings for indexed canonical domains and aliases.
	// Use fmt.Sprintf(LabelDomainCanonical, index) to get "turkis.domain.<index>"
	LabelDomainCanonical = "turkis.domain.%d"
	// Use fmt.Sprintf(LabelDomainAlias, domainIndex, aliasIndex) to get "turkis.domain.<domainIndex>.alias.<aliasIndex>"
	LabelDomainAlias = "turkis.domain.%d.alias.%d"
)

type ContainerLabels struct {
	AppName         string
	DeploymentID    string
	Ignore          bool
	HealthCheckPath string
	ACMEEmail       string
	Port            string
	Domains         []Domain
}

// Parse from docker labels to ContainerLabels struct.
func ParseContainerLabels(labels map[string]string) (*ContainerLabels, error) {
	cl := &ContainerLabels{
		AppName:      labels[LabelAppName],
		DeploymentID: labels[LabelDeploymentID],
		ACMEEmail:    labels[LabelACMEEmail],
	}

	// Parse and validate Ignore flag.
	if v, ok := labels[LabelIgnore]; ok {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: %w", LabelIgnore, err)
		}
		cl.Ignore = b
	}

	if v, ok := labels[LabelPort]; ok {
		cl.Port = v
	} else {
		cl.Port = DefaultContainerPort
	}

	// Set HealthCheckPath with default value.
	if v, ok := labels[LabelHealthCheckPath]; ok {
		cl.HealthCheckPath = v
	} else {
		cl.HealthCheckPath = DefaultHealthCheckPath
	}

	// Parse domains
	domainMap := make(map[int]*Domain)

	// Process domain and alias labels.
	for key, value := range labels {
		if !strings.HasPrefix(key, "turkis.domain.") {
			continue
		}
		if strings.Contains(key, ".alias.") {
			// Parse alias key: "turkis.domain.<domainIdx>.alias.<aliasIdx>"
			var domainIdx, aliasIdx int
			if _, err := fmt.Sscanf(key, LabelDomainAlias, &domainIdx, &aliasIdx); err != nil {
				// Skip keys that don't conform.
				continue
			}
			domain := getOrCreateDomain(domainMap, domainIdx)
			domain.Aliases = append(domain.Aliases, value)
		} else {
			// Parse canonical domain key: "turkis.domain.<domainIdx>"
			var domainIdx int
			if _, err := fmt.Sscanf(key, LabelDomainCanonical, &domainIdx); err != nil {
				continue
			}
			domain := getOrCreateDomain(domainMap, domainIdx)
			domain.Canonical = value
		}
	}

	// Build the sorted slice of domains.
	var indices []int
	for i := range domainMap {
		indices = append(indices, i)
	}
	sort.Ints(indices)
	for _, i := range indices {
		cl.Domains = append(cl.Domains, *domainMap[i])
	}

	// Optional: validate the parsed labels.
	if err := cl.IsValid(); err != nil {
		return nil, err
	}

	return cl, nil
}

// getOrCreateDomain returns an existing *config.Domain from domainMap or creates a new one.
func getOrCreateDomain(domainMap map[int]*Domain, idx int) *Domain {
	if domain, exists := domainMap[idx]; exists {
		return domain
	}
	domainMap[idx] = &Domain{}
	return domainMap[idx]
}

// ToLabels converts the ContainerLabels struct back to a map[string]string.
func (cl *ContainerLabels) ToLabels() map[string]string {
	labels := map[string]string{
		LabelAppName:         cl.AppName,
		LabelDeploymentID:    cl.DeploymentID,
		LabelIgnore:          strconv.FormatBool(cl.Ignore),
		LabelHealthCheckPath: cl.HealthCheckPath,
		LabelPort:            cl.Port,
		LabelACMEEmail:       cl.ACMEEmail,
	}

	// Iterate through the domains slice.
	for i, domain := range cl.Domains {
		// Set canonical domain.
		canonicalKey := fmt.Sprintf(LabelDomainCanonical, i)
		labels[canonicalKey] = domain.Canonical

		// Set aliases.
		for j, alias := range domain.Aliases {
			aliasKey := fmt.Sprintf(LabelDomainAlias, i, j)
			labels[aliasKey] = alias
		}
	}

	return labels
}

// We assume that all labels need to be present for the labels to be valid.
func (cl *ContainerLabels) IsValid() error {
	if cl.AppName == "" {
		return fmt.Errorf("appName is required")
	}
	if cl.DeploymentID == "" {
		return fmt.Errorf("deploymentID is required")
	}

	if cl.ACMEEmail == "" {
		return fmt.Errorf("ACME email is required")
	}

	if !helpers.IsValidEmail(cl.ACMEEmail) {
		return fmt.Errorf("ACME email is not valid")
	}

	if cl.Port == "" {
		return fmt.Errorf("port is required")
	}

	if len(cl.Domains) == 0 {
		return fmt.Errorf("at least one domain is required")
	}
	return nil
}

func (cl *ContainerLabels) String() string {
	bold := color.New(color.Bold).SprintFunc()
	yellow := color.New(color.FgHiYellow).SprintFunc()
	cyan := color.New(color.FgHiCyan).SprintFunc()

	var builder strings.Builder
	// Create a tabwriter with padding settings.
	w := tabwriter.NewWriter(&builder, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "%s:\t%s\n", yellow("App Name"), cyan(cl.AppName))
	fmt.Fprintf(w, "%s:\t%s\n", yellow("Deployment ID"), cyan(cl.DeploymentID))
	fmt.Fprintf(w, "%s:\t%t\n", yellow("Ignore"), cl.Ignore)
	fmt.Fprintf(w, "%s:\t%s\n", yellow("Health Check Path"), cyan(cl.HealthCheckPath))
	fmt.Fprintf(w, "%s:\t%s\n", yellow("ACME Email"), cyan(cl.ACMEEmail))
	fmt.Fprintf(w, "%s:\t%s\n", yellow("Port"), cyan(cl.Port))

	fmt.Fprintln(w, yellow("Domains:"))
	for i, domain := range cl.Domains {
		fmt.Fprintf(w, "\t%s\t%s\n", bold(fmt.Sprintf("Domain %d", i+1)), cyan(domain.Canonical))
		if len(domain.Aliases) > 0 {
			fmt.Fprintf(w, "\t%s\t%s\n", yellow("Aliases"), cyan(strings.Join(domain.Aliases, ", ")))
		}
	}
	w.Flush()

	return builder.String()
}
