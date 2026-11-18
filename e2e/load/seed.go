package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/util/version"

	"github.com/fundament-oss/fundament/common/auth"
	organizationv1 "github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1"
	"github.com/fundament-oss/fundament/organization-api/pkg/proto/gen/v1/organizationv1connect"
)

const (
	// participantsName is the participants file in the output directory, as
	// `run` hands it to k6.
	participantsName = "participants.json"
	teamPrefix       = "load-"
	// How long the seeded sessions and API keys last: a day of runs.
	credentialTTL = 24 * time.Hour
)

// SeedCmd seeds organizations load-NN of PER admins (load-NN-K@fieldlab.test),
// each with one cluster and node pool, and an API key per participant.
// Seeding again reuses what exists and issues new API keys, since a key is
// shown only once.
type SeedCmd struct {
	Teams           int    `arg:"" optional:"" default:"20" help:"Organizations."`
	Per             int    `arg:"" optional:"" default:"5" help:"Participants per organization."`
	Out             string `default:"reports/load" help:"Directory for the participants, as run reads them." type:"path"`
	Funops          string `required:"" help:"Path of a built funops binary." type:"existingfile"`
	DBHost          string `default:"localhost:54328" help:"host:port of the database funops connects to."`
	OrganizationAPI string `default:"https://organization.fundament.localhost:8443" help:"organization-api URL."`
}

// participant is one entry of participants.json, as agents.js reads it.
type participant struct {
	Email     string `json:"email"`
	OrgID     string `json:"orgId"`
	JWT       string `json:"jwt"`
	Cluster   string `json:"cluster"`
	ClusterID string `json:"clusterId"`
	APIKeyID  string `json:"apiKeyId"`
	APIKey    string `json:"apiKey"`
}

type seeder struct {
	funops      string
	databaseURL string
	secret      []byte
	api         string
	transport   http.RoundTripper
}

func (c *SeedCmd) Run(ctx context.Context, k kube) error {
	if c.Teams < 1 || c.Per < 1 {
		return fmt.Errorf("need at least one team of one participant, not %d of %d", c.Teams, c.Per)
	}
	if err := k.local(ctx); err != nil {
		return err
	}
	password, err := k.secret(ctx, "db-fun-operator", "password")
	if err != nil {
		return err
	}
	// Participants have no password login, so the seed signs their sessions
	// with authn-api's secret.
	secret, err := k.env(ctx, "authn-api", "JWT_SECRET")
	if err != nil {
		return err
	}
	if secret == "" {
		return errors.New("authn-api has no JWT_SECRET")
	}
	transport, err := localTransport(ctx)
	if err != nil {
		return err
	}
	s := seeder{
		funops:      c.Funops,
		databaseURL: (&url.URL{Scheme: "postgresql", User: url.UserPassword("fun_operator", password), Host: c.DBHost, Path: "/fundament"}).String(),
		secret:      []byte(secret),
		api:         c.OrganizationAPI,
		transport:   transport,
	}
	people, err := s.seed(ctx, c.Teams, c.Per)
	if err != nil {
		return err
	}
	// The sessions and keys k6 uses; the file is the user's own, under the
	// gitignored reports directory.
	data, err := json.Marshal(people) //nolint:gosec // see above
	if err != nil {
		return fmt.Errorf("encoding participants: %w", err)
	}
	if err := os.MkdirAll(c.Out, 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", c.Out, err)
	}
	out := filepath.Join(c.Out, participantsName)
	if err := os.WriteFile(out, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", out, err)
	}
	fmt.Printf("%d participants in %s; their API keys and sessions expire in %s\n", len(people), out, credentialTTL)
	return nil
}

// localTransport trusts the local deployment's mkcert CA.
func localTransport(ctx context.Context) (http.RoundTripper, error) {
	root, err := output(ctx, "mkcert", "-CAROOT")
	if err != nil {
		return nil, err
	}
	pem, err := os.ReadFile(root + "/rootCA.pem") //nolint:gosec // mkcert's own CA directory
	if err != nil {
		return nil, fmt.Errorf("reading the mkcert CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		return nil, errors.New("the mkcert CA holds no certificate")
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	return t, nil
}

func (s *seeder) seed(ctx context.Context, teams, per int) ([]participant, error) {
	var orgs []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := s.run(ctx, &orgs, "organization", "list"); err != nil {
		return nil, err
	}
	existing := map[string]string{}
	for _, o := range orgs {
		existing[o.Name] = o.ID
	}

	var people []participant
	var known *offer
	for t := 1; t <= teams; t++ {
		org := fmt.Sprintf("%s%02d", teamPrefix, t)
		team, err := s.team(ctx, org, existing[org], per, &known)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", org, err)
		}
		people = append(people, team...)
		fmt.Printf("%s: %d participants, cluster %s\n", org, len(team), team[0].ClusterID)
	}
	return people, nil
}

// offer is what the platform offers a new cluster.
type offer struct {
	region, machineType, version string
}

func (s *seeder) team(ctx context.Context, org, organizationID string, per int, known **offer) ([]participant, error) {
	if organizationID == "" {
		var created struct {
			ID string `json:"id"`
		}
		if err := s.run(ctx, &created, "organization", "create", org); err != nil {
			return nil, err
		}
		if err := s.run(ctx, nil, "organization", "quota", "set-clusters", org, "5"); err != nil {
			return nil, err
		}
		organizationID = created.ID
	}
	orgID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization id %q: %w", organizationID, err)
	}

	members, err := s.members(ctx, org)
	if err != nil {
		return nil, err
	}
	// Only the first PER: a smaller reseed leaves the others out.
	emails := make([]string, per)
	for i := range emails {
		emails[i] = fmt.Sprintf("%s-%d@fieldlab.test", org, i+1)
		if _, ok := members[emails[i]]; !ok {
			if err := s.run(ctx, nil, "organization", "member", "add", org, emails[i], "--permission", "admin", "--create-user"); err != nil {
				return nil, err
			}
		}
	}
	if members, err = s.members(ctx, org); err != nil {
		return nil, err
	}
	people := make([]participant, 0, per)
	for _, email := range emails {
		userID, err := uuid.Parse(members[email])
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", email, err)
		}
		token, err := s.session(userID, orgID, email, time.Now())
		if err != nil {
			return nil, err
		}
		people = append(people, participant{Email: email, OrgID: organizationID, JWT: token})
	}

	first := people[0].JWT
	clusters := organizationv1connect.NewClusterServiceClient(s.client(first, organizationID), s.api)
	if *known == nil {
		regions, err := retry(ctx, "ListRegions", transient, func(ctx context.Context) (*organizationv1.ListRegionsResponse, error) {
			return clusters.ListRegions(ctx, &organizationv1.ListRegionsRequest{})
		})
		if err != nil {
			return nil, err
		}
		if *known, err = pickOffer(regions.GetRegions()); err != nil {
			return nil, err
		}
	}
	o := *known
	if err := s.run(ctx, nil, "organization", "quota", "set-nodes", org, o.region, o.machineType, "100"); err != nil {
		return nil, err
	}

	cluster := org + "-c"
	clusterID, err := ensureCluster(ctx, clusters, cluster, o)
	if err != nil {
		return nil, err
	}
	for i := range people {
		p := &people[i]
		keys := organizationv1connect.NewAPIKeyServiceClient(s.client(p.JWT, organizationID), s.api)
		key, err := retry(ctx, "CreateAPIKey", refused, func(ctx context.Context) (*organizationv1.CreateAPIKeyResponse, error) {
			return keys.CreateAPIKey(ctx, organizationv1.CreateAPIKeyRequest_builder{
				Name:      "agent-" + strconv.FormatInt(time.Now().UnixNano(), 36),
				ExpiresIn: credentialTTL.String(),
			}.Build())
		})
		if err != nil {
			return nil, err
		}
		p.Cluster, p.ClusterID, p.APIKeyID, p.APIKey = cluster, clusterID, key.GetId(), key.GetToken()
	}
	return people, nil
}

// pickOffer takes the first region, its first machine type and its newest
// Kubernetes version.
func pickOffer(regions []*organizationv1.Region) (*offer, error) {
	if len(regions) == 0 || len(regions[0].GetMachineTypes()) == 0 {
		return nil, errors.New("the platform offers no region with machine types")
	}
	r := regions[0]
	var newest *version.Version
	var name string
	for _, v := range r.GetKubernetesVersions() {
		parsed, err := version.Parse(v)
		if err != nil {
			continue
		}
		if newest == nil || newest.LessThan(parsed) {
			newest, name = parsed, v
		}
	}
	if newest == nil {
		return nil, fmt.Errorf("region %s offers no Kubernetes version: %v", r.GetName(), r.GetKubernetesVersions())
	}
	return &offer{region: r.GetName(), machineType: r.GetMachineTypes()[0].GetName(), version: name}, nil
}

func ensureCluster(ctx context.Context, clusters organizationv1connect.ClusterServiceClient, name string, o *offer) (string, error) {
	list, err := retry(ctx, "ListClusters", transient, func(ctx context.Context) (*organizationv1.ListClustersResponse, error) {
		return clusters.ListClusters(ctx, &organizationv1.ListClustersRequest{})
	})
	if err != nil {
		return "", err
	}
	var id string
	for _, c := range list.GetClusters() {
		if c.GetName() == name {
			id = c.GetId()
		}
	}
	if id == "" {
		created, err := retry(ctx, "CreateCluster", refused, func(ctx context.Context) (*organizationv1.CreateClusterResponse, error) {
			return clusters.CreateCluster(ctx, organizationv1.CreateClusterRequest_builder{Name: name, Region: o.region, KubernetesVersion: o.version}.Build())
		})
		if err != nil {
			return "", err
		}
		id = created.GetClusterId()
	}

	// A seed cut short after the cluster gets its node pool on the next one.
	pools, err := retry(ctx, "ListNodePools", transient, func(ctx context.Context) (*organizationv1.ListNodePoolsResponse, error) {
		return clusters.ListNodePools(ctx, organizationv1.ListNodePoolsRequest_builder{ClusterId: id}.Build())
	})
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(pools.GetNodePools(), func(p *organizationv1.NodePool) bool { return p.GetName() == "pool" }) {
		return id, nil
	}
	_, err = retry(ctx, "CreateNodePool", refused, func(ctx context.Context) (*organizationv1.CreateNodePoolResponse, error) {
		return clusters.CreateNodePool(ctx, organizationv1.CreateNodePoolRequest_builder{
			ClusterId: id, Name: "pool", MachineType: o.machineType, AutoscaleMin: 1, AutoscaleMax: 3,
		}.Build())
	})
	return id, err
}

// session signs the claims authn-api puts in a session token.
func (s *seeder) session(userID, organizationID uuid.UUID, name string, now time.Time) (string, error) {
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    auth.ConsoleIssuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{auth.TokenTypeUser},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(credentialTTL)),
		},
		OrganizationIDs: []uuid.UUID{organizationID},
		Name:            name,
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("signing a session: %w", err)
	}
	return signed, nil
}

// client sends every request as the given session, for the given organization.
func (s *seeder) client(token, organizationID string) *http.Client {
	return &http.Client{Timeout: 30 * time.Second, Transport: identity{base: s.transport, token: token, organizationID: organizationID}}
}

type identity struct {
	base           http.RoundTripper
	token          string
	organizationID string
}

func (i identity) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+i.token)
	r.Header.Set(auth.OrganizationHeader, i.organizationID)
	return i.base.RoundTrip(r) //nolint:wrapcheck // a RoundTripper passes its transport's error on
}

// retry repeats a call for up to a minute while again(err) holds: a new
// organization's grants reach OpenFGA through the authz outbox, so a call may
// be refused or unavailable for a moment.
func retry[T any](ctx context.Context, what string, again func(error) bool, call func(context.Context) (T, error)) (T, error) {
	deadline := time.Now().Add(time.Minute)
	for {
		v, err := call(ctx)
		if err == nil {
			return v, nil
		}
		if !again(err) || time.Now().After(deadline) {
			return v, fmt.Errorf("%s: %w", what, err)
		}
		select {
		case <-ctx.Done():
			return v, fmt.Errorf("%s: %w", what, ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// refused holds for a create refused before it acted: by authorization, or
// by the circuit breaker or proxy. After any other error the object may
// exist, and creating it again would add a second one.
func refused(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodePermissionDenied, connect.CodeUnavailable:
		return true
	default:
		return false
	}
}

// transient holds for a read that may succeed when tried again.
func transient(err error) bool {
	switch connect.CodeOf(err) {
	case connect.CodePermissionDenied, connect.CodeUnavailable, connect.CodeUnknown, connect.CodeInternal, connect.CodeDeadlineExceeded:
		return true
	default:
		return false
	}
}

// members maps each member's email to their user id.
func (s *seeder) members(ctx context.Context, org string) (map[string]string, error) {
	var list []struct {
		Email  string `json:"email"`
		UserID string `json:"user_id"`
	}
	if err := s.run(ctx, &list, "organization", "member", "list", org); err != nil {
		return nil, err
	}
	members := make(map[string]string, len(list))
	for _, m := range list {
		members[m.Email] = m.UserID
	}
	return members, nil
}

// run runs funops with JSON output, decoding it into out unless out is nil.
func (s *seeder) run(ctx context.Context, out any, args ...string) error {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, s.funops, append([]string{"-o", "json"}, args...)...) //nolint:gosec // the funops binary the recipe built
	cmd.Env = append(os.Environ(), "DATABASE_URL="+s.databaseURL)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("funops %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	if out == nil || len(bytes.TrimSpace(stdout.Bytes())) == 0 {
		return nil
	}
	if err := json.Unmarshal(stdout.Bytes(), out); err != nil {
		return fmt.Errorf("funops %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
