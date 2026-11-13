package selfupdate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/testing/ca"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// fakeBundle is what the fake releases serve as the signature over sums.
// It stands in for a Sigstore bundle, which needs a real Sigstore to make;
// checkSignature is tested against a virtual one below.
func fakeBundle(sums []byte) []byte {
	sum := sha256.Sum256(sums)
	return []byte("signed:" + hex.EncodeToString(sum[:]))
}

// trustFakeBundles makes Install accept fakeBundle signatures, and only
// those, for the rest of the test.
func trustFakeBundles(t *testing.T) {
	t.Helper()
	old := verifySums
	verifySums = func(_ context.Context, sums, b []byte) error {
		if !bytes.Equal(b, fakeBundle(sums)) {
			return errors.New("bad signature")
		}
		return nil
	}
	t.Cleanup(func() { verifySums = old })
}

func TestInstallNeedsASignature(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", "new", false)
	ctx := context.Background()
	rel, err := Latest(ctx, srv.Client(), srv.URL+"/latest")
	if err != nil {
		t.Fatal(err)
	}
	var unsigned []Asset
	for _, a := range rel.Assets {
		if a.Name != bundleName {
			unsigned = append(unsigned, a)
		}
	}
	rel.Assets = unsigned
	exe := exeFile(t)
	if err := Install(ctx, srv.Client(), rel, exe); err == nil || !strings.Contains(err.Error(), "not signed") {
		t.Errorf("Install of an unsigned release: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Errorf("binary was replaced by an unsigned release: %q", b)
	}
}

func TestInstallRefusesABadSignature(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", "new", false)
	ctx := context.Background()
	rel, err := Latest(ctx, srv.Client(), srv.URL+"/latest")
	if err != nil {
		t.Fatal(err)
	}
	verifySums = func(context.Context, []byte, []byte) error { return errors.New("bad signature") }
	exe := exeFile(t)
	if err := Install(ctx, srv.Client(), rel, exe); err == nil || !strings.Contains(err.Error(), "bad signature") {
		t.Errorf("Install with a bad signature: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "old" {
		t.Errorf("binary was replaced despite the bad signature: %q", b)
	}
}

// A virtual Sigstore issues certificates, signs and logs like the real one,
// so checkSignature runs the real verification against it.
func TestCheckSignature(t *testing.T) {
	vs, err := ca.NewVirtualSigstore()
	if err != nil {
		t.Fatal(err)
	}
	sums := []byte("aa  " + AssetName("v0.5.0", runtime.GOOS, runtime.GOARCH) + "\n")
	opts := []verify.VerifierOption{verify.WithTransparencyLog(1), verify.WithObserverTimestamps(1)}
	sign := func(identity, issuer string) verify.SignedEntity {
		t.Helper()
		e, err := vs.Sign(identity, issuer, sums)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	if err := checkSignature(sign(signerIdentity, signerIssuer), sums, vs, opts...); err != nil {
		t.Errorf("signature by the release workflow: %v", err)
	}
	for _, c := range []struct{ what, identity, issuer string }{
		{"another branch", strings.Replace(signerIdentity, "refs/heads/main", "refs/heads/evil", 1), signerIssuer},
		{"a pull request", strings.Replace(signerIdentity, "refs/heads/main", "refs/pull/1/merge", 1), signerIssuer},
		{"another workflow", strings.Replace(signerIdentity, "release.yml", "other.yml", 1), signerIssuer},
		{"another repository", strings.Replace(signerIdentity, "foolscap-folio", "fork", 1), signerIssuer},
		{"another issuer", signerIdentity, "https://accounts.example.com"},
	} {
		if err := checkSignature(sign(c.identity, c.issuer), sums, vs, opts...); err == nil {
			t.Errorf("signature from %s was accepted", c.what)
		}
	}
	tampered := append([]byte("bb  evil.zip\n"), sums...)
	if err := checkSignature(sign(signerIdentity, signerIssuer), tampered, vs, opts...); err == nil {
		t.Error("signature over different checksums was accepted")
	}
}

func TestVerifySigstoreRejectsGarbage(t *testing.T) {
	if err := verifySigstore(context.Background(), []byte("x"), []byte("not json")); err == nil || !strings.Contains(err.Error(), "not a Sigstore bundle") {
		t.Errorf("verifySigstore on garbage: %v", err)
	}
}
