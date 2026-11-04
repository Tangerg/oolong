package arch

import (
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// These packages are where application-owned state, callbacks and composition meet
// a frame. Lower grid and text primitives may mutate the frame they are handed.
var renderingPackages = []string{
	"ui/core/content",
	"ui/base",
	"ui/kit",
	"extensions/latex",
	"extensions/markdown",
}

// The gate follows local bodies and known external effect declarations. Dynamic
// application interfaces retain their own contracts; this is not a whole-program
// proof of observational purity.
func TestRenderingCannotReachIntrinsicEffects(t *testing.T) {
	root := repoRoot(t)
	for _, directory := range renderingPackages {
		t.Run(strings.ReplaceAll(directory, "/", "_"), func(t *testing.T) {
			model := loadRenderPackage(t, filepath.Join(root, directory), directory)
			for _, entry := range model.functions {
				if entry.receiver == "" || (entry.name != "HeightForWidth" && !strings.HasPrefix(entry.name, "Draw")) {
					continue
				}
				for _, finding := range model.effectsFrom(entry) {
					t.Errorf("%s: %s reaches %s through %s", finding.position,
						entry.identity(), finding.effect, strings.Join(finding.path, " -> "))
				}
			}
		})
	}
}

type renderEffect struct {
	position token.Position
	effect   string
	path     []string
}

func (m renderPackageModel) effectsFrom(entry *renderFunction) []renderEffect {
	reachable := m.reachableFrom(entry)
	var findings []renderEffect
	seen := make(map[struct {
		position token.Pos
		effect   string
	}]bool)
	for _, function := range reachable.functions {
		record := func(position token.Pos, effect string) {
			key := struct {
				position token.Pos
				effect   string
			}{position, effect}
			if effect == "" || seen[key] {
				return
			}
			seen[key] = true
			findings = append(findings, renderEffect{
				position: function.set.Position(position), effect: effect,
				path: slices.Clone(reachable.paths[function]),
			})
		}
		inspectRenderBody(function.body, func(node ast.Node) {
			if goroutine, ok := node.(*ast.GoStmt); ok {
				record(goroutine.Go, "a goroutine")
			}
			if call := m.invocation(node); call != nil {
				direct, callbacks := reachable.callTargets(call)
				direct.include(callbacks)
				for target := range direct {
					record(call.Pos(), m.targetEffect(target))
				}
			}
		})
	}
	slices.SortFunc(findings, func(a, b renderEffect) int {
		if byPosition := strings.Compare(a.position.String(), b.position.String()); byPosition != 0 {
			return byPosition
		}
		return strings.Compare(a.effect, b.effect)
	})
	return findings
}

func (m renderPackageModel) targetEffect(target renderTarget) string {
	if target.callback != nil {
		return "the " + string(m.callbackByObject[target.callback].phase) + " callback " + target.callback.Name()
	}
	function := target.function
	if function == nil || function.Pkg() == nil {
		return ""
	}
	if effect := effectPackageCall(function.Pkg().Path(), function.Name()); effect != "" {
		return effect
	}
	if effect := effectOwnedCall(function); effect != "" {
		return effect
	}
	if function.Pkg() == m.checkedPackage && m.functionByObject[function] == nil && !interfaceMethod(function) {
		return "an unresolved package-local call " + function.Name()
	}
	return ""
}

func interfaceMethod(function *types.Func) bool {
	receiver := function.Type().(*types.Signature).Recv()
	if receiver == nil {
		return false
	}
	_, isInterface := receiver.Type().Underlying().(*types.Interface)
	return isInterface
}

func effectPackageCall(path, name string) string {
	switch path {
	case "bufio", "io", "io/fs", "net", "net/http", "os", "os/exec", "syscall":
		return "I/O through " + path + "." + name
	case "log", "log/slog":
		return "logging through " + path + "." + name
	case "crypto/rand", "math/rand", "math/rand/v2":
		return "nondeterminism through " + path + "." + name
	case "fmt":
		if strings.HasPrefix(name, "Print") || strings.HasPrefix(name, "Fprint") {
			return "output through fmt." + name
		}
	case "time":
		switch name {
		case "After", "AfterFunc", "NewTicker", "NewTimer", "Now", "Since", "Sleep", "Tick", "Until":
			return "clock or scheduling work through time." + name
		}
	}
	return ""
}

func effectOwnedCall(function *types.Func) string {
	receiver := function.Type().(*types.Signature).Recv()
	if receiver == nil {
		return ""
	}
	typ := types.Unalias(receiver.Type())
	if pointer, ok := typ.(*types.Pointer); ok {
		typ = types.Unalias(pointer.Elem())
	}
	named, ok := typ.(*types.Named)
	if !ok {
		return ""
	}
	owner := named.Origin().Obj()
	if owner.Pkg() == nil {
		return ""
	}
	if publicationMethod(owner.Pkg().Path(), owner.Name(), function.Name()) {
		return "the publication operation " + owner.Pkg().Path() + "." + owner.Name() + "." + function.Name()
	}
	return ""
}

// Each rule names a declaring capability, not a spelling that unrelated pure
// methods may share. Local model mutations are covered by behavioral purity tests.
func publicationMethod(path, receiver, method string) bool {
	switch path {
	case modulePath + "/ui/core/publication":
		if receiver == "Publisher" {
			switch method {
			case "SetGeometry", "TryPublish", "Commit", "Written", "Fail", "FailAdmission", "Close", "CloseAdmission", "Subscribe", "FlushNotifications":
				return true
			}
		}
	case modulePath + "/ui/core/program":
		switch receiver {
		case "Publisher":
			return method == "TryPublish" || method == "Subscribe"
		case "Dispatcher":
			return method == "Post" || method == "Invoke"
		case "Session":
			switch method {
			case "ReportDirectory", "Hand", "Suspend", "SetTitle", "SetProgress", "Bell", "Notify":
				return true
			}
		case "Runtime":
			switch method {
			case "After", "Every", "Start", "Refresh", "Quit":
				return true
			}
		case "Clipboard":
			return method == "Copy" || method == "Read"
		case "Images":
			return method == "Transmit" || method == "Release"
		case "Scene":
			return method == "Invoke"
		}
	case modulePath + "/ui/core/scene":
		return receiver == "Prepared" && method == "Commit" || receiver == "Root" && (method == "Invoke" || method == "FlushNotifications")
	case modulePath + "/ui/core/grid":
		return (receiver == "Screen" || receiver == "Inline") && method == "Flush" || receiver == "Output" && method == "Commit"
	case modulePath + "/ui/core/term":
		if receiver == "Writer" || receiver == "FrameSink" || receiver == "Terminal" {
			switch method {
			case "Queue", "QueueControl", "Drain", "Copy", "ReadClipboard", "Transmit", "ReleaseImage", "SetTitle", "SetProgress", "Bell", "Notify":
				return true
			}
		}
	case modulePath + "/ui/base":
		return receiver == "TranscriptPublisher" && (method == "TryPublish" || method == "Subscribe")
	}
	return false
}
