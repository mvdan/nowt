// Copyright (c) 2020, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode"
)

var goEnv struct {
	GOPROXY   string
	GONOPROXY string // TODO: use it to avoid GOPROXY
}

func init() {
	out, err := exec.Command("go", "env", "-json").CombinedOutput()
	if err != nil {
		println(string(out))
		log.Fatal(err)
	}
	if err := json.Unmarshal(out, &goEnv); err != nil {
		log.Fatal(err)
	}
}

// TODO: is "all" the right pattern here? it seems to include modules that
// aren't actually needed as per "go mod why -m". maybe we need a narrower set
// of modules.

func main() {
	mods, err := listModules(context.TODO(), nil, "all")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, mod := range mods {
		if mod.Main {
			continue // that's us
		}
		size, err := fetchSize(mod.Path, mod.Version)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			// don't exit; "410 Gone" errors are common
		}
		fmt.Printf("%s@%s %d\n", mod.Path, mod.Version, size)
	}
}

// TODO: do we just trust the proxy to not return a bad zip? can we verify
// against go.sum without downloading the entire thing?

// TODO: do we obey replace directives? we probably should, as 'go mod download'
// does too.

func fetchSize(path, version string) (int64, error) {
	for _, proxy := range strings.Split(goEnv.GOPROXY, ",") {
		if !strings.HasPrefix(proxy, "http://") && !strings.HasPrefix(proxy, "https://") {
			// TODO: support file://, probably
			return 0, fmt.Errorf("only http and https proxies are supported: %q", proxy)
		}

		zipURL := proxy + "/" + caseEncode(path) + "/@v/" + caseEncode(version) + ".zip"
		resp, err := http.Head(zipURL)
		if code := resp.StatusCode; err == nil && code >= 300 {
			err = fmt.Errorf("%d %s", code, http.StatusText(code))
		}
		if err != nil {
			log.Printf("HEAD %s: %v", zipURL, err)
			continue // try another proxy
		}
		resp.Body.Close()
		return resp.ContentLength, nil
	}
	return 0, fmt.Errorf("did not find %s@%s on any proxy", path, version)
}

// caseEncode turns "fooBar" into "foo!bar".
func caseEncode(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteByte('!')
			b.WriteRune(unicode.ToLower(r))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

type Module struct {
	Path      string       // module path
	Version   string       // module version
	Versions  []string     // available module versions (with -versions)
	Replace   *Module      // replaced by this module
	Time      *time.Time   // time version was created
	Update    *Module      // available update, if any (with -u)
	Main      bool         // is this the main module?
	Indirect  bool         // is this module only an indirect dependency of main module?
	Dir       string       // directory holding files for this module, if any
	GoMod     string       // path to go.mod file used when loading this module, if any
	GoVersion string       // go version used in module
	Error     *ModuleError // error loading module
}

type ModuleError struct {
	Err string // the error itself
}

func getEnv(env []string, name string) string {
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 && name == kv[:i] {
			return kv[i+1:]
		}
	}
	return ""
}

// listModules is eavily adapted from listPackages in gofumpt/gen.go.
func listModules(ctx context.Context, env []string, args ...string) (mods []*Module, finalErr error) {
	goArgs := append([]string{"list", "-m", "-json", "-e"}, args...)
	cmd := exec.CommandContext(ctx, "go", goArgs...)
	cmd.Env = env
	cmd.Dir = getEnv(env, "PWD")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	defer func() {
		if finalErr != nil && stderrBuf.Len() > 0 {
			// TODO: wrap? but the format is backwards, given that
			// stderr is likely multi-line
			finalErr = fmt.Errorf("%v\n%s", finalErr, stderrBuf.Bytes())
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(stdout)
	for dec.More() {
		var mod Module
		if err := dec.Decode(&mod); err != nil {
			return nil, err
		}
		mods = append(mods, &mod)
	}
	if err := cmd.Wait(); err != nil {
		return nil, err
	}
	return mods, nil
}
