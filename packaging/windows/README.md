# Windows packaging

folio reaches Windows users three ways, all built by the release workflow:

- **winget**, Windows' own package manager: `winget install ChristophBerger.FoolscapFolio`.
  GoReleaser writes the manifest (the `winget` section of `.goreleaser.yaml`)
  and opens a pull request to [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs).
- **The setup program**, `folio_<tag>_windows_setup.exe`, for people who would
  rather double-click: [`folio.iss`](folio.iss), built with
  [Inno Setup](https://jrsoftware.org/isinfo.php) by
  [`windows-setup.yml`](../../.github/workflows/windows-setup.yml).
- **Scoop**, as before.

## The setup program

It installs for the current user, into `%LOCALAPPDATA%\Programs\Foolscap Folio`,
without administrator rights, and

- picks the amd64 or arm64 binary for the machine,
- puts its folder on the user's `PATH`, so `folio` works in a new terminal,
- adds a **Foolscap Folio** entry to the Start menu, which runs `folio web` in
  a console window starting in Documents (the window shows the address to open;
  closing it stops folio web),
- offers to start folio web when it finishes,
- registers an uninstaller under Installed apps, which takes it all off again.

Because the folder is the user's own, `folio upgrade` and the Update button keep
such an install up to date. A newer setup program installs over an older one.

To build it by hand on Windows, with Inno Setup 6.3 or newer installed:

```powershell
$env:CGO_ENABLED = 0; $env:GOOS = "windows"
foreach ($a in "amd64", "arm64") { $env:GOARCH = $a; go build -trimpath -ldflags "-s -w -X main.version=v0.0.0" -o build\$a\folio.exe ./cmd/folio }
& "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe" /DAppVersion=0.0.0 packaging\windows\folio.iss
```

The result is in `installer\`.

## One-time setup

Nothing below is needed for a release to go out: until each piece is in place,
that part is built and skipped.

### winget

1. Fork [microsoft/winget-pkgs](https://github.com/microsoft/winget-pkgs) into
   the `christophberger-ailab` organization, keeping the name `winget-pkgs`.
2. Create a **classic** personal access token with the `public_repo` scope
   (a fine-grained token cannot open a pull request on a repository outside
   its owner), and store it as the repository secret `WINGET_TOKEN`.
3. After the next release, look for the pull request in winget-pkgs. The first
   one asks you to sign Microsoft's CLA with a comment, and a moderator reviews
   a new package by hand, which can take a few days. Later versions go through
   the automated checks alone, usually within a day.

### Code signing with SignPath

An unsigned setup program makes Windows SmartScreen say "Windows protected your
PC" and hide the Run button behind "More info". The
[SignPath Foundation](https://signpath.org) signs open-source projects for free.

1. Apply at signpath.org for the project. They check that it is open source
   with an OSI license (BSD-3-Clause is), maintained, and built from its public
   repository; they ask for a code signing policy on the project's page, naming
   who may approve a signing request.
2. Once accepted, in SignPath:
   - create the project with the slug `foolscap-folio`,
   - add GitHub as a trusted build system for it,
   - give it an artifact configuration that signs the setup program inside the
     artifact zip:

     ```xml
     <artifact-configuration xmlns="http://signpath.io/artifact-configuration/v1">
       <zip-file>
         <pe-file path="folio_*_windows_setup.exe">
           <authenticode-sign/>
         </pe-file>
       </zip-file>
     </artifact-configuration>
     ```

   - and a signing policy with the slug `release-signing`.
3. In the repository's settings, add the secret `SIGNPATH_API_TOKEN` (an API
   token of a SignPath CI user that may submit to the project) and the variable
   `SIGNPATH_ORGANIZATION_ID`. Other slugs than the two above go in the
   variables `SIGNPATH_PROJECT_SLUG` and `SIGNPATH_SIGNING_POLICY_SLUG`.

From then on every release's setup program is signed before it is published.
Pull request builds are never signed.
