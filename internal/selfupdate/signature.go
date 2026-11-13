package selfupdate

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

// bundleName is the release asset holding the Sigstore bundle for
// SHA256SUMS.txt: the signature, the short-lived certificate naming the
// workflow that made it, and its transparency log entry. The release
// workflow writes it with `cosign sign-blob --bundle`.
const bundleName = checksumsName + ".sigstore.json"

// maxBundle limits the bundle download; a real one is a few kilobytes.
const maxBundle = 256 << 10

// The identity a release's signing certificate must name. Releases are
// signed keylessly from GitHub Actions: the certificate's subject is the
// workflow file at the ref it ran on, and only release.yml on main may
// sign. A release signed from any other workflow, branch or repository is
// refused, even when its signature is otherwise valid.
const (
	signerIdentity = "https://github.com/christophberger-ailab/foolscap-folio/.github/workflows/release.yml@refs/heads/main"
	signerIssuer   = "https://token.actions.githubusercontent.com"
)

// verifySums checks that bundle is a valid signature over sums by the
// release workflow. It is a variable so that tests can stand in for
// Sigstore.
var verifySums = verifySigstore

// verifySigstore verifies against Sigstore's public instance. Its trusted
// root (the Fulcio CA and Rekor keys) comes over TUF, anchored in the TUF
// root sigstore-go embeds, and is cached for a day.
func verifySigstore(ctx context.Context, sums, bundleJSON []byte) error {
	var b bundle.Bundle
	if err := b.UnmarshalJSON(bundleJSON); err != nil {
		return fmt.Errorf("%s: not a Sigstore bundle: %w", bundleName, err)
	}
	opts := tuf.DefaultOptions().WithContext(ctx).WithCacheValidity(1)
	if dir, err := os.UserCacheDir(); err == nil {
		opts = opts.WithCachePath(filepath.Join(dir, "folio", "sigstore-tuf"))
	}
	tr, err := root.FetchTrustedRootWithOptions(opts)
	if err != nil {
		return fmt.Errorf("cannot fetch Sigstore's trusted root to check the release signature: %w", err)
	}
	return checkSignature(&b, sums, tr,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithTransparencyLog(1),
		verify.WithObserverTimestamps(1),
	)
}

// checkSignature verifies entity as a signature over sums by the release
// workflow, against the given trusted material.
func checkSignature(entity verify.SignedEntity, sums []byte, tm root.TrustedMaterial, opts ...verify.VerifierOption) error {
	v, err := verify.NewVerifier(tm, opts...)
	if err != nil {
		return err
	}
	id, err := verify.NewShortCertificateIdentity(signerIssuer, "", signerIdentity, "")
	if err != nil {
		return err
	}
	policy := verify.NewPolicy(verify.WithArtifact(bytes.NewReader(sums)), verify.WithCertificateIdentity(id))
	if _, err := v.Verify(entity, policy); err != nil {
		return fmt.Errorf("%s is not signed by folio's release workflow: %w", checksumsName, err)
	}
	return nil
}
