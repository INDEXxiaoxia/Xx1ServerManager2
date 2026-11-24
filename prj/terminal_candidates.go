package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// Each terminal owns its SSH connection. Match its foreground shell using
// OpenSSH's connection tuple, and enumerate through its live working directory.
const directoryCandidatesScript = `import os, sys, base64, json
connection = b"SSH_CONNECTION=" + os.environ["SSH_CONNECTION"].encode()
prefix = base64.b64decode(sys.argv[1]).decode()
offset, limit = map(int, sys.argv[2:])
shells = []
for pid in os.listdir("/proc"):
    if not pid.isdecimal(): continue
    proc = "/proc/" + pid
    try:
        with open(proc + "/stat") as f: stat = f.read().rsplit(")", 1)[1].split()
        if stat[4] == "0" or stat[2] != stat[5] or pid != stat[2]: continue
        if os.path.basename(os.readlink(proc + "/exe")) not in ("bash","zsh","dash","sh","ksh","ash"): continue
        with open(proc + "/environ","rb") as f:
            if connection in f.read().split(b"\0"): shells.append(proc)
    except (OSError, ValueError): continue
if len(shells) != 1: sys.exit(1)
names = []
with os.scandir(shells[0] + "/cwd") as entries:
    for entry in entries:
        name = entry.name
        if not name.startswith(prefix) or not name.isprintable(): continue
        try:
            if not entry.is_dir(): continue
        except OSError: continue
        if offset: offset -= 1; continue
        names.append(name)
        if len(names) > limit: break
print(json.dumps(names))
`

func (a *App) TerminalDirectoryCandidates(id, prefix string, offset int) ([]string, error) {
	if offset < 0 || offset >= 70 || len(prefix) > 1024 || !utf8.ValidString(prefix) {
		return nil, errors.New("无效的目录查询")
	}
	a.terminalMu.Lock()
	ts := a.terminals[id]
	a.terminalMu.Unlock()
	if ts == nil {
		return nil, errors.New("终端已关闭")
	}
	ts.directoryMu.RLock()
	defer ts.directoryMu.RUnlock()
	ts.mu.Lock()
	closed := ts.closed
	ts.mu.Unlock()
	if closed {
		return nil, errors.New("终端已关闭")
	}
	limit := 21
	if 70-offset < limit {
		limit = 70 - offset
	}
	cmd := fmt.Sprintf("python3 - '%s' %d %d <<'XX1_PY'\n%s\nXX1_PY", base64.StdEncoding.EncodeToString([]byte(prefix)), offset, limit, directoryCandidatesScript)
	code, out, _ := ts.client.ExecCommandLimited(cmd, 8, false)
	var names []string
	if code != 0 || json.Unmarshal([]byte(out), &names) != nil {
		return nil, errors.New("无法读取当前 shell 目录，请使用命令方式切换")
	}
	return names, nil
}
