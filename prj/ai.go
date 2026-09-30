package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"ServerManager2/internal/aiclient"
	"ServerManager2/internal/ansi"
	"ServerManager2/internal/models"
)

// ---- 终端 AI ----
//
// 三个模式（详见 .trae/documents/terminal-ai-design.md）：
//   suggest  建议：只给命令，用户自己复制/插入到终端，绝不自动执行
//   exec     直接执行：AI 把这一条命令注入当前可见终端、回读结果，然后停下
//   takeover 完全接管：自动连续多轮，直到 AI 认为完成或达到最大轮数
//
// 关键设计：命令注入到**用户看得见的那个终端**里（与手敲完全一样），
// 靠一段隐藏标记回读"这条命令的输出与退出码"；整个过程中页面不会被遮罩盖住。

const (
	aiKeyBaseURL      = "ai.baseURL"
	aiKeyAPIKey       = "ai.apiKey" // 密文（用主密码 / 本机密钥加密）
	aiKeyModel        = "ai.model"
	aiKeyMaxRounds    = "ai.maxRounds"
	aiKeyTimeout      = "ai.timeout"
	aiKeyContextLines = "ai.contextLines"

	aiDefaultMaxRounds    = 20
	aiDefaultTimeout      = 60
	aiDefaultContextLines = 200

	// 单步硬超时：到点发 Ctrl+C 收尾，避免终端被一个跑飞的命令长期占住
	aiStepHardTimeout = 5 * time.Minute
	// 静默多久后提示"可能卡住/正在长时间运行"
	aiStepStallWarn = 120 * time.Second
	// 轮询间隔：既用于等标记，也用于读控制指令
	aiPollInterval = 300 * time.Millisecond
	// 单步回读上限 / 送给模型的终端内容上限
	aiStepMaxOutput   = 64 * 1024
	aiContextMaxBytes = 32 * 1024
	// 对话历史里保留几轮的终端原文（更早的只留一行摘要，防止上下文越滚越大）
	aiHistoryKeepRaw = 2
	// 建议 / 直接执行模式保留的对话消息条数（一问一答算两条）
	aiSuggestHistoryMax = 16
)

// ---- 配置读写 ----

// GetAISettings 读取 AI 配置。API Key 只回"是否已保存"，不回明文。
func (a *App) GetAISettings() (models.AISettings, error) {
	base, _ := a.store.GetSetting(aiKeyBaseURL)
	key, _ := a.store.GetSetting(aiKeyAPIKey)
	model, _ := a.store.GetSetting(aiKeyModel)

	s := models.AISettings{
		BaseURL:      base,
		Model:        model,
		APIKeySet:    strings.TrimSpace(key) != "",
		MaxRounds:    aiDefaultMaxRounds,
		Timeout:      aiDefaultTimeout,
		ContextLines: aiDefaultContextLines,
	}
	if v, _ := a.store.GetSetting(aiKeyMaxRounds); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.MaxRounds = n
		}
	}
	if v, _ := a.store.GetSetting(aiKeyTimeout); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.Timeout = n
		}
	}
	if v, _ := a.store.GetSetting(aiKeyContextLines); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.ContextLines = n
		}
	}
	return s, nil
}

// SaveAISettings 保存 AI 配置。
// apiKey 留空表示"不改动已保存的 Key"（避免用户只是改了模型名就把 Key 清掉）。
func (a *App) SaveAISettings(s models.AISettings) error {
	if a.store == nil {
		return errors.New("存储尚未初始化")
	}
	set := func(k, v string) error { return a.store.SetSetting(k, v) }
	if err := set(aiKeyBaseURL, strings.TrimSpace(s.BaseURL)); err != nil {
		return err
	}
	if err := set(aiKeyModel, strings.TrimSpace(s.Model)); err != nil {
		return err
	}
	if err := set(aiKeyMaxRounds, strconv.Itoa(clampInt(s.MaxRounds, 1, 100, aiDefaultMaxRounds))); err != nil {
		return err
	}
	if err := set(aiKeyTimeout, strconv.Itoa(clampInt(s.Timeout, 5, 600, aiDefaultTimeout))); err != nil {
		return err
	}
	if err := set(aiKeyContextLines, strconv.Itoa(clampInt(s.ContextLines, 20, 2000, aiDefaultContextLines))); err != nil {
		return err
	}
	if k := strings.TrimSpace(s.APIKey); k != "" {
		if a.crypto == nil || !a.crypto.HasMasterKey() {
			return errors.New("加密密钥不可用，请先解锁后再保存 API Key")
		}
		enc, err := a.crypto.EncryptPassword(k)
		if err != nil {
			return err
		}
		if err := set(aiKeyAPIKey, enc); err != nil {
			return err
		}
	}
	return nil
}

func clampInt(v, min, max, def int) int {
	if v <= 0 {
		return def
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// aiClient 组装 AI 客户端（解密 API Key）
func (a *App) aiClient() (*aiclient.Client, models.AISettings, error) {
	s, err := a.GetAISettings()
	if err != nil {
		return nil, s, err
	}
	if strings.TrimSpace(s.BaseURL) == "" {
		return nil, s, errors.New("尚未配置 AI 接口地址：设置 → AI 设置")
	}
	if strings.TrimSpace(s.Model) == "" {
		return nil, s, errors.New("尚未配置模型名：设置 → AI 设置")
	}
	key := ""
	if enc, _ := a.store.GetSetting(aiKeyAPIKey); enc != "" {
		if a.crypto == nil || !a.crypto.HasMasterKey() {
			return nil, s, errors.New("应用处于锁定状态，无法读取 API Key（请解锁后重试）")
		}
		plain, err := a.crypto.DecryptPassword(enc)
		if err != nil {
			return nil, s, errors.New("API Key 解密失败，请在设置里重新填写")
		}
		key = plain
	}
	return aiclient.New(aiclient.Config{
		BaseURL: s.BaseURL,
		APIKey:  key,
		Model:   s.Model,
		Timeout: time.Duration(s.Timeout) * time.Second,
	}), s, nil
}

// TestAIConnection 测试连通：验证地址 / Key / 模型名三件事
func (a *App) TestAIConnection() (models.AITestResult, error) {
	c, s, err := a.aiClient()
	res := models.AITestResult{Endpoint: aiclient.Endpoint(s.BaseURL), Model: s.Model}
	if err != nil {
		res.Message = err.Error()
		return res, nil
	}
	ms, err := c.Ping()
	res.LatencyMs = ms
	if err != nil {
		res.Message = err.Error()
		return res, nil
	}
	res.OK = true
	res.Message = "连通成功"
	return res, nil
}

// ---- 终端内容快照 ----

// AITerminalSnapshot 取当前终端内容（供前端"预览将发送什么"用）
func (a *App) AITerminalSnapshot(termID, mode string, lines int, selection string) (models.AIContext, error) {
	ts := a.getTerminal(termID)
	if ts == nil {
		return models.AIContext{}, errors.New("终端会话不存在（可能已关闭）")
	}
	return a.buildContext(ts, mode, lines, selection), nil
}

// buildContext 组装给 AI 看的终端内容。
// 档位：selection（用户选区，优先级最高）| last50 | last200 | sinceLast | all
func (a *App) buildContext(ts *TerminalSession, mode string, lines int, selection string) models.AIContext {
	if lines <= 0 {
		s, _ := a.GetAISettings()
		lines = s.ContextLines
		if lines <= 0 {
			lines = aiDefaultContextLines
		}
	}
	if strings.TrimSpace(selection) != "" {
		mode = "selection"
	}
	ctx := models.AIContext{TermID: ts.ID, Mode: mode}

	if mode == "selection" {
		text := ansi.Strip(selection)
		ctx.Text = strings.TrimSpace(text)
		if ctx.Text == "" {
			ctx.Empty = true
		}
		ctx.Lines = countLines(ctx.Text)
		ctx.Bytes = len(text)
		return ctx
	}

	from := int64(0)
	if mode == "sinceLast" {
		from = ts.aiSeq
	}
	raw, fromActual, to, dropped := ts.tail.snapshot(from)
	ctx.Dropped = dropped
	ctx.SeqFrom = fromActual
	ctx.SeqTo = to

	clean := ansi.Strip(string(raw))
	before := clean
	clean = ansi.FoldRepeats(clean)
	if clean != before {
		ctx.Elided = true
	}
	if mode != "all" {
		clean = ansi.TailLines(clean, lines)
	}
	elided, truncated := ansi.Elide(clean, aiContextMaxBytes)
	ctx.Truncated = truncated
	ctx.Text = strings.TrimSpace(elided)
	ctx.Lines = countLines(ctx.Text)
	ctx.Bytes = len(elided)
	ctx.Empty = ctx.Text == ""
	return ctx
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

func (a *App) getTerminal(id string) *TerminalSession {
	a.terminalMu.Lock()
	defer a.terminalMu.Unlock()
	return a.terminals[id]
}

// ---- 提示词 ----

// aiSuggestSystemPrompt 建议 / 直接执行模式的系统提示
func aiSuggestSystemPrompt(serverName, host string) string {
	return "你是 Xx1ServerManager2 里的 Linux 运维助手，服务对象是一名正在通过 SSH 终端操作服务器 " +
		serverName + "（" + host + "）的运维人员。\n" +
		"用户会附带终端当前内容，你要基于它回答问题。\n\n" +
		"输出要求：只输出一个 JSON 对象，不要任何额外文字、不要代码围栏：\n" +
		"{\"text\":\"给用户看的回答（中文，可含少量 markdown）\",\"commands\":[{\"command\":\"可执行的单行命令\",\"reason\":\"这条命令做什么、有什么风险\"}]}\n" +
		"规则：\n" +
		"1. commands 里最多给 3 条，按推荐顺序排；不确定就给 1 条。不需要命令时给空数组。\n" +
		"2. 命令必须是单行、非交互的（不要 vim/top/less/nano）；分页类命令加 --no-pager、安装类加 -y。\n" +
		"3. reason 里说清楚这条命令的作用与影响（尤其是有删除/重启/安装行为时）。\n" +
		"4. 不要编造不存在的服务名、路径或文件名；不确定就让用户先跑一条只读命令确认。"
}

// aiTakeoverSystemPrompt 完全接管模式的系统提示
func aiTakeoverSystemPrompt(serverName, host string) string {
	return "你是 Xx1ServerManager2 里的 Linux 运维助手，正在通过 SSH 交互式终端**直接操作**服务器 " +
		serverName + "（" + host + "）。你每轮给出的一条命令会被真实执行，执行结果会回给你，你据此决定下一步。\n\n" +
		"输出要求：只输出一个 JSON 对象，不要任何额外文字、不要代码围栏：\n" +
		"{\"thought\":\"给用户看的一句话说明（中文）\",\"command\":\"要执行的命令\",\"finish\":false,\"summary\":\"\"}\n" +
		"规则：\n" +
		"1. command 只放一条命令，可用 && 或 ; 串联；必须是单行可执行的。\n" +
		"2. 绝对不要用交互式命令（vim/top/less/nano/more/交互式 mysql 等）；分页命令加 --no-pager。\n" +
		"3. 需要提权时先执行 `sudo -n true` 检查免密 sudo；不可用就不要用 sudo，改为 finish=true 并说明需要用户手动提权。\n" +
		"4. 任务完成时 finish=true、command 留空，并在 summary 里写一句总结（做了什么、结果如何）。\n" +
		"5. 遇到报错先读错误信息再决定下一步，不要重复执行同样的失败命令；连续两次失败就 finish=true 说明卡在哪。\n" +
		"6. 缺少信息时先用只读命令侦察（cat /etc/os-release、uname -a、command -v xxx、ls、df -h）。\n" +
		"7. 严禁破坏性操作：rm -rf /、mkfs、改分区、改防火墙、改 sshd 配置、清空 /etc 下文件。\n" +
		"8. 安装类任务尽量用系统包管理器（apt-get/yum/dnf），并加 -y；下载安装脚本时说明来源。"
}

// ---- 建议 / 直接执行模式 ----

// AIChat 建议 / 直接执行模式的一次问答。
//
// 会话默认带上下文：同一个 sessionId 的对话历史会被带进下一次请求
// （终端内容**不进历史**——每次都用最新的，避免上下文被旧屏幕内容撑爆）。
// 「重置对话」会丢掉会话，随后就是全新的一段。
func (a *App) AIChat(req models.AIRequest) (models.AIResponse, error) {
	resp := models.AIResponse{SessionID: req.SessionID}
	start := time.Now()

	if strings.TrimSpace(req.Text) == "" {
		return resp, errors.New("请输入你想让 AI 做什么")
	}
	sv, err := a.getServerWithPassword(req.ServerID)
	if err != nil {
		return resp, err
	}
	ts := a.getTerminal(req.TermID)
	if ts == nil {
		return resp, errors.New("终端会话不存在（可能已关闭）")
	}
	client, _, err := a.aiClient()
	if err != nil {
		return resp, err
	}

	ctx := a.buildContext(ts, req.Context, req.Lines, req.Selection)
	resp.Context = ctx

	mode := req.Mode
	if mode != "exec" {
		mode = "suggest"
	}
	s := a.ensureAISession(req.SessionID, req.TermID, sv.ID, sv.Name, mode, 1, "")
	s.mu.Lock()
	s.mode = mode
	hist := make([]aiclient.Message, len(s.history))
	copy(hist, s.history)
	s.mu.Unlock()
	resp.SessionID = s.id

	userContent := "【我的问题】\n" + strings.TrimSpace(req.Text)
	if ctx.Text != "" {
		userContent += "\n\n【终端输出】\n" + ctx.Text
	} else {
		userContent += "\n\n【终端输出】\n（为空）"
	}

	msgs := make([]aiclient.Message, 0, len(hist)+2)
	msgs = append(msgs, aiclient.Message{Role: "system", Content: aiSuggestSystemPrompt(sv.Name, sv.Host)})
	msgs = append(msgs, hist...)
	msgs = append(msgs, aiclient.Message{Role: "user", Content: userContent})

	reply, err := client.Chat(msgs)
	resp.Model = reply.Model
	resp.Truncated = reply.Truncated
	resp.ElapsedMs = time.Since(start).Milliseconds()
	if err != nil {
		return resp, err
	}

	// 只把"问题与回答"记进历史（终端屏幕内容每次重新取，不进历史）
	s.pushHistory(aiclient.Message{Role: "user", Content: "【我的问题】\n" + strings.TrimSpace(req.Text)})
	s.pushHistory(aiclient.Message{Role: "assistant", Content: reply.Text})
	s.trimHistory(aiSuggestHistoryMax)

	text, cmds := parseAISuggestReply(reply.Text)
	resp.Text = text
	for _, c := range cmds {
		level, reason := riskOf(c.Command)
		if c.Reason != "" {
			reason = c.Reason
		}
		resp.Commands = append(resp.Commands, models.AICommand{Command: c.Command, Risk: level, Reason: reason})
	}
	if resp.Commands == nil {
		resp.Commands = []models.AICommand{}
	}
	return resp, nil
}

// AIExecOne 直接执行模式：把一条命令注入当前可见终端，回读结果后停下。
// 也用于建议模式下用户点「执行这条」。
// 执行结果会记进会话历史，所以下一条消息里 AI 知道"刚才那条命令跑出了什么"。
func (a *App) AIExecOne(termID, sessionID, command string) (models.AIExecResult, error) {
	ts := a.getTerminal(termID)
	if ts == nil {
		return models.AIExecResult{}, errors.New("终端会话不存在（可能已关闭）")
	}
	if strings.TrimSpace(command) == "" {
		return models.AIExecResult{}, errors.New("命令为空")
	}
	// 单条执行也登记成一个会话：这样"停止 / 继续等待 / Ctrl+C"同样可用。
	// 会话留着不删——它的 sid 与前端那段对话绑定，执行结果要进历史供下一轮参考；
	// 清理由「重置对话」(AIReset)、关闭终端、以及开新对话时的空闲会话回收负责。
	s := a.ensureAISession(sessionID, termID, "", "", "exec", 1, "")
	s.mu.Lock()
	s.cancelled = false
	s.paused = false
	s.control = make(chan string, 8)
	if s.mode == "" {
		s.mode = "exec"
	}
	s.mu.Unlock()
	res := a.aiExecStep(s, command)
	// 把结果喂回上下文：退出码 + 输出（输出按大块折叠，只保留最近两轮原文）
	s.pushHistory(aiclient.Message{
		Role: "user",
		Content: fmt.Sprintf("【终端输出】\n我执行了：%s\n退出码：%d，状态：%s\n%s",
			command, res.ExitCode, res.Status, res.Output),
	})
	s.compressHistory()
	return res, nil
}

// ---- 接管模式 ----

// AIStartTakeover 启动完全接管。
// req.ContinueSession 为真时，复用同一个 sessionID 的上下文继续（用于"追加任务"）。
func (a *App) AIStartTakeover(req models.AIRequest) (string, error) {
	if strings.TrimSpace(req.Text) == "" {
		return "", errors.New("请输入要让 AI 完成的任务")
	}
	sv, err := a.getServerWithPassword(req.ServerID)
	if err != nil {
		return "", err
	}
	ts := a.getTerminal(req.TermID)
	if ts == nil {
		return "", errors.New("终端会话不存在（可能已关闭）")
	}
	if _, _, err := a.aiClient(); err != nil {
		return "", err
	}
	settings, _ := a.GetAISettings()
	maxRounds := req.MaxRounds
	if maxRounds <= 0 {
		maxRounds = settings.MaxRounds
	}
	maxRounds = clampInt(maxRounds, 1, 100, aiDefaultMaxRounds)

	// 同一个 sessionID 会命中已有会话（同一段对话里继续提问/追加任务）。此时必须把状态复位：
	// 上一次 stop() 留下的 cancelled=true 不清掉的话，新的循环会刚起步就退出。
	// round 归零 = 重新拿到 maxRounds 轮预算（"已达上限"之后追加任务可以接着跑）。
	s := a.ensureAISession(req.SessionID, req.TermID, sv.ID, sv.Name, "takeover", maxRounds, "")
	s.mu.Lock()
	s.mode = "takeover"
	s.maxRounds = maxRounds
	s.cancelled = false
	s.paused = false
	s.lastBlock = ""
	s.round = 0
	s.control = make(chan string, 8)
	s.state = "running"
	s.mu.Unlock()
	if !req.ContinueSession {
		// 这是一段新对话（不是追加）：同一终端上已经停下、没人再听的旧会话顺手清掉，
		// 免得反复"发送"把旧上下文一直堆在内存里
		a.dropIdleSessionsForTerm(req.TermID, s.id)
	}
	s.appendUserTask(req.Text)

	go a.runTakeover(s)
	return s.id, nil
}

// dropIdleSessionsForTerm 清掉某终端上"已经停下"的会话（正在跑的留着）
func (a *App) dropIdleSessionsForTerm(termID, keepID string) {
	a.aiMu.Lock()
	hit := make([]*aiSession, 0, 2)
	for id, s := range a.aiSessions {
		if id == keepID || s.termID != termID {
			continue
		}
		s.mu.Lock()
		idle := s.state != "running"
		s.mu.Unlock()
		if idle {
			hit = append(hit, s)
		}
	}
	a.aiMu.Unlock()
	for _, s := range hit {
		a.dropAISession(s.id)
	}
}

// AIStopTakeover 停止接管（正在等待的那一步会立刻收尾）
func (a *App) AIStopTakeover(sessionID string) error {
	s := a.existingAISession(sessionID)
	if s == nil {
		return nil
	}
	s.stop()
	return nil
}

// AITakeoverControl 接管过程中的交互控制：
//
//	continue — 用户已手动处理完终端里的提示（输了密码、退出了 vim），继续等这一步
//	abort    — 中止整个任务（等价于「停止」）
func (a *App) AITakeoverControl(sessionID, action string) error {
	s := a.existingAISession(sessionID)
	if s == nil {
		return nil
	}
	switch action {
	case "continue":
		s.mu.Lock()
		s.paused = false
		s.lastBlock = ""
		s.mu.Unlock()
		select {
		case s.control <- "continue":
		default:
		}
	case "abort":
		s.stop()
	default:
		return errors.New("未知的控制指令：" + action)
	}
	return nil
}

// AIReset 清空某个终端的 AI 上下文（"清空对话 / 新建对话"）
func (a *App) AIReset(termID string) error {
	a.aiMu.Lock()
	ids := make([]string, 0, 4)
	for id, s := range a.aiSessions {
		if termID == "" || s.termID == termID {
			ids = append(ids, id)
		}
	}
	a.aiMu.Unlock()
	for _, id := range ids {
		if s := a.existingAISession(id); s != nil {
			s.stop()
		}
		a.dropAISession(id)
	}
	return nil
}

// dropAISessionsForTerm 终端关闭时清掉挂在它上面的 AI 会话（先在跑的会一起停掉）
func (a *App) dropAISessionsForTerm(termID string) {
	a.aiMu.Lock()
	hit := make([]*aiSession, 0, 2)
	for _, s := range a.aiSessions {
		if s.termID == termID {
			hit = append(hit, s)
		}
	}
	a.aiMu.Unlock()
	for _, s := range hit {
		s.stop()
		a.dropAISession(s.id)
	}
}

// closeAllAISessions 停止全部 AI 会话（主密码锁定时调用）
func (a *App) closeAllAISessions() {
	a.aiMu.Lock()
	all := make([]*aiSession, 0, len(a.aiSessions))
	for _, s := range a.aiSessions {
		all = append(all, s)
	}
	a.aiMu.Unlock()
	for _, s := range all {
		s.stop()
	}
}

// ---- 会话对象 ----

// aiSession 一次 AI 交互的上下文。
// 对话记录只活在内存里（按需求不落库），应用重启即消失。
type aiSession struct {
	id         string
	termID     string
	serverID   string
	serverName string
	mode       string
	maxRounds  int
	userTask   string

	mu        sync.Mutex
	history   []aiclient.Message
	rawSteps  []string // 最近的终端原文（更早的会被压缩成摘要行）
	round     int
	state     string
	cancelled bool
	paused    bool
	lastBlock string
	control   chan string
}

func (s *aiSession) isCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled
}

func (s *aiSession) stop() {
	s.mu.Lock()
	s.cancelled = true
	s.state = "stopped"
	s.mu.Unlock()
	select {
	case s.control <- "abort":
	default:
	}
}

// appendUserTask 把用户要求写进对话历史（追加任务时就是新的一条 user 消息）
func (s *aiSession) appendUserTask(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userTask = text
	s.history = append(s.history, aiclient.Message{Role: "user", Content: text})
}

// pushHistory 追加一条历史消息，并把较早的终端原文压缩掉
func (s *aiSession) pushHistory(m aiclient.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = append(s.history, m)
}

// trimHistory 只保留最近 n 条消息（建议/直接执行模式的问答历史无限增长没有意义）。
// 裁完如果开头是 assistant，再丢一条——避免历史以"回答"开头让模型错位。
func (s *aiSession) trimHistory(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || len(s.history) <= n {
		return
	}
	s.history = append([]aiclient.Message{}, s.history[len(s.history)-n:]...)
	if len(s.history) > 0 && s.history[0].Role == "assistant" {
		s.history = s.history[1:]
	}
}

// compressHistory 只保留最近 aiHistoryKeepRaw 轮的终端原文，其余换成一行摘要。
// 这是多轮接管不把上下文撑爆的关键。
func (s *aiSession) compressHistory() {
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := 0
	for i := len(s.history) - 1; i >= 0; i-- {
		m := s.history[i]
		if m.Role != "user" || !strings.Contains(m.Content, "【终端输出】") {
			continue
		}
		kept++
		if kept <= aiHistoryKeepRaw {
			continue
		}
		head := m.Content
		if idx := strings.Index(head, "【终端输出】"); idx >= 0 {
			head = head[:idx]
		}
		s.history[i] = aiclient.Message{
			Role:    "user",
			Content: head + fmt.Sprintf("【终端输出】\n（较早的一轮，%d 字节，已折叠以节省上下文）", len(m.Content)),
		}
	}
}

func (a *App) ensureAISession(id, termID, serverID, serverName, mode string, maxRounds int, _ string) *aiSession {
	if strings.TrimSpace(id) == "" {
		id = fmt.Sprintf("ai-%d", time.Now().UnixNano())
	}
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	if s, ok := a.aiSessions[id]; ok {
		return s
	}
	s := &aiSession{
		id: id, termID: termID, serverID: serverID, serverName: serverName,
		mode: mode, maxRounds: maxRounds, state: "running",
		control: make(chan string, 8),
	}
	a.aiSessions[id] = s
	return s
}

func (a *App) existingAISession(id string) *aiSession {
	a.aiMu.Lock()
	defer a.aiMu.Unlock()
	return a.aiSessions[id]
}

func (a *App) dropAISession(id string) {
	a.aiMu.Lock()
	delete(a.aiSessions, id)
	a.aiMu.Unlock()
}

// ---- 事件 ----

func (a *App) emitAIState(s *aiSession, state, message string) {
	a.emitAIStateKind(s, state, "", message)
}

// emitAIStateKind 同上，并带上暂停原因分类（前端据此决定是否把键盘交还给用户）
func (a *App) emitAIStateKind(s *aiSession, state, kind, message string) {
	s.mu.Lock()
	s.state = state
	round := s.round
	maxRounds := s.maxRounds
	s.mu.Unlock()
	runtime.EventsEmit(a.ctx, "ai:state:"+s.id, models.AIState{
		SessionID: s.id, TermID: s.termID, State: state, Kind: kind,
		Round: round, MaxRounds: maxRounds, Message: message, At: time.Now().UnixMilli(),
	})
}

func (a *App) emitAIStep(s *aiSession, step models.AIStep) {
	runtime.EventsEmit(a.ctx, "ai:step:"+s.id, step)
}

// ---- 命令注入与回读 ----

// buildStepInject 构造"命令 + 隐藏结束标记"的注入文本。
//
// 标记形如 X1S<stepID>D<退出码>X1E，末尾用 \r + ESC[K 擦掉，用户看不见但原始字节里有。
// 关键点：printf 的格式串里**不能出现完整标记**——否则终端回显的命令行本身就会命中标记，
// 导致我们把"命令还没跑"误判成"已经跑完"。所以标记用 %s 拼出来：
//
//	printf '\nX1%sS%sD%sX1E\r\033[K' '' '<stepID>' "$?"
//
// 回显里只有 X1%sS%sD%s 这种碎片，永远拼不出完整标记。
func buildStepInject(stepID, command string) (string, string) {
	key := "X1S" + stepID + "D"
	markerTail := "X1E"
	printfLine := "printf '\\nX1%sS%sD%s" + markerTail + "\\r\\033[K' '' '" + stepID + "' \"$?\""

	cmd := strings.TrimRight(command, " \t\n")
	if !strings.Contains(cmd, "\n") {
		// 单行：直接接在命令后面
		return cmd + " ; " + printfLine + "\r", key
	}
	// 多行：标记另起一行（$? 仍然是上一条命令的退出码）
	return cmd + "\n" + printfLine + "\r", key
}

// findMarker 在清洗后的文本里找结束标记，返回标记位置与退出码。
// 会跳过那些"后面不是数字"的伪命中（例如用户在终端里恰好回显了类似内容）。
func findMarker(text, key string) (int, int, bool) {
	searchFrom := 0
	for {
		i := strings.Index(text[searchFrom:], key)
		if i < 0 {
			return 0, 0, false
		}
		i += searchFrom
		rest := text[i+len(key):]
		j := strings.Index(rest, "X1E")
		if j >= 0 {
			if code, err := strconv.Atoi(strings.TrimSpace(rest[:j])); err == nil {
				return i, code, true
			}
		}
		searchFrom = i + len(key)
		if searchFrom >= len(text) {
			return 0, 0, false
		}
	}
}

// stripEcho 把回显的命令行与前后空行从输出里去掉，让交给模型的内容尽量干净
func stripEcho(output, command string) string {
	lines := strings.Split(output, "\n")
	// 去掉首尾空行
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	// 第一行如果是命令回显（含命令前 30 个字符）就丢掉
	if len(lines) > 0 && len(command) > 0 {
		probe := strings.TrimSpace(command)
		if len(probe) > 30 {
			probe = probe[:30]
		}
		first := strings.TrimSpace(lines[0])
		if probe != "" && strings.Contains(first, probe) {
			lines = lines[1:]
		}
	}
	return strings.Join(lines, "\n")
}

// aiExecStep 注入一条命令并等它跑完（接管 / 单条执行共用）。
func (a *App) aiExecStep(s *aiSession, command string) models.AIExecResult {
	start := time.Now()
	level, reason := riskOf(command)
	res := models.AIExecResult{Command: command, ExitCode: -1, Risk: level, Message: reason}

	ts := a.getTerminal(s.termID)
	if ts == nil {
		res.Status = "error"
		res.Message = "终端会话不存在（可能已关闭）"
		return res
	}

	stepID := randomStepID()
	inject, key := buildStepInject(stepID, command)
	from := ts.tail.end()
	// 记录"自上次 AI 命令以来"这一档的起点
	ts.mu.Lock()
	ts.aiSeq = from
	ts.mu.Unlock()

	if err := a.TerminalWrite(s.termID, inject); err != nil {
		res.Status = "error"
		res.Message = "写入终端失败：" + err.Error()
		return res
	}

	deadline := time.Now().Add(aiStepHardTimeout)
	lastChange := time.Now()
	lastTo := from
	stallWarned := false

	for {
		if s.isCancelled() {
			// 中止：把命令打断，别让它在服务器上继续跑
			_ = a.TerminalWrite(s.termID, "\x03")
			res.Status = "cancelled"
			res.Message = "已中止（已向终端发送 Ctrl+C）"
			return finishStep(a, s, ts, res, from, start)
		}
		if time.Now().After(deadline) {
			_ = a.TerminalWrite(s.termID, "\x03")
			data, _, _, _ := ts.tail.snapshot(from)
			out := stripEcho(ansi.Strip(string(data)), command)
			out, _ = ansi.Elide(out, aiStepMaxOutput)
			res.Output = out
			res.Status = "timeout"
			res.Message = fmt.Sprintf("命令超过 %s 仍在运行，已发送 Ctrl+C 收尾", aiStepHardTimeout)
			return finishStep(a, s, ts, res, from, start)
		}

		data, _, to, dropped := ts.tail.snapshot(from)
		rawText := string(data)
		clean := ansi.Strip(rawText)
		if dropped {
			// 起点已被环形缓冲丢掉：拿不到完整输出，如实说明
			res.Message = "输出过多，起点已被缓冲区覆盖（只回读到最近一段）"
		}
		// 注意：标记必须在**原始字节流**里找。
		// 标记行末尾是 \r + ESC[K（把它从用户眼前擦掉），而 ansi.Strip 会把
		// "单独的 \r 覆盖行"整行丢弃——在清洗后的文本里永远找不到标记。
		if idx, code, ok := findMarker(rawText, key); ok {
			out := stripEcho(ansi.Strip(rawText[:idx]), command)
			out, truncated := ansi.Elide(out, aiStepMaxOutput)
			res.ExitCode = code
			res.Output = out
			res.Truncated = truncated
			res.Status = "done"
			if code != 0 {
				res.Message = fmt.Sprintf("命令退出码 %d", code)
			}
			return finishStep(a, s, ts, res, from, start)
		}

		if to != lastTo {
			lastTo = to
			lastChange = time.Now()
			stallWarned = false
		}

		// 交互式程序 / 等待输入：暂停并把键盘交还给用户，
		// 之后命令跑完仍会打出标记，所以循环继续等（用户处理完点「输入完成，继续」）
		blockReason, blockKind := "", ""
		if ansi.HasAltScreen(rawText) {
			blockKind = "altscreen"
			blockReason = "命令进入了全屏交互程序（vim / top / less 之类）。请在终端里退出它（通常按 q），然后点下面的「输入完成，继续」"
		} else if pat, isPrompt := ansi.LooksLikePrompt(clean); isPrompt {
			blockKind = "input"
			if looksLikePasswordPrompt(pat) {
				blockReason = "命令需要你输入密码（检测到提示：" + pat + "）。终端已解锁：请直接输入或粘贴密码，" +
					"按回车提交——提交后 AI 会自动继续；若没有继续，再点「输入完成，继续」"
			} else {
				blockReason = "命令在等你确认（检测到提示：" + pat + "）。终端已解锁：请输入 y / n 或需要的答案并回车，" +
					"提交后 AI 会自动继续；若没有继续，再点「输入完成，继续」"
			}
		} else if !stallWarned && time.Since(lastChange) > aiStepStallWarn {
			stallWarned = true
			blockKind = "stall"
			blockReason = fmt.Sprintf("已经 %s 没有新输出：可能仍在长时间运行，也可能卡住了。"+
				"可以继续等；确认卡死就点「停止」把任务停下", aiStepStallWarn)
		}
		if blockReason != "" {
			s.mu.Lock()
			already := s.lastBlock
			s.lastBlock = blockReason
			s.paused = true
			s.mu.Unlock()
			if already != blockReason {
				a.emitAIStateKind(s, "paused", blockKind, blockReason)
			}
		}

		select {
		case act := <-s.control:
			switch act {
			case "continue":
				s.mu.Lock()
				s.paused = false
				s.lastBlock = ""
				s.mu.Unlock()
				lastChange = time.Now()
				stallWarned = false
				a.emitAIState(s, "running", "已继续等待命令结果")
			case "abort":
				_ = a.TerminalWrite(s.termID, "\x03")
				res.Status = "cancelled"
				res.Message = "已中止（已向终端发送 Ctrl+C）"
				return finishStep(a, s, ts, res, from, start)
			}
		case <-time.After(aiPollInterval):
		}
	}
}

// finishStep 补齐耗时并落一条审计记录
func finishStep(a *App, s *aiSession, ts *TerminalSession, res models.AIExecResult, from int64, start time.Time) models.AIExecResult {
	res.ElapsedMs = time.Since(start).Milliseconds()
	if a.store != nil {
		status := models.TaskSuccess
		if res.Status != "done" || res.ExitCode != 0 {
			status = models.TaskFailed
		}
		serverName := s.serverName
		_ = a.store.AddCommandHistory(&models.CommandHistory{
			ServerID: s.serverID, ServerName: serverName, Command: res.Command,
			Status: status, ExitCode: res.ExitCode,
			Stdout: res.Output, Stderr: res.Message,
			Duration: res.ElapsedMs, CreatedAt: time.Now(),
			Source: "ai", AISessionID: s.id,
		})
	}
	return res
}

// ---- 接管主循环 ----

func (a *App) runTakeover(s *aiSession) {
	client, settings, err := a.aiClient()
	if err != nil {
		a.emitAIState(s, "error", err.Error())
		return
	}
	ts := a.getTerminal(s.termID)
	if ts == nil {
		a.emitAIState(s, "error", "终端会话不存在（可能已关闭）")
		return
	}

	// 先确认 shell 支持我们用的这套 POSIX 语法（fish/csh 不支持 $? 与 printf 的写法）
	if probe := a.probeShell(s); probe != "" {
		a.emitAIState(s, "error", probe)
		return
	}

	a.emitAIState(s, "running", fmt.Sprintf("AI 接管中（最多 %d 轮），可随时停止", s.maxRounds))

	for {
		if s.isCancelled() {
			a.emitAIState(s, "stopped", "已停止")
			return
		}
		s.mu.Lock()
		round := s.round + 1
		maxRounds := s.maxRounds
		s.mu.Unlock()
		if round > maxRounds {
			a.emitAIState(s, "maxrounds",
				fmt.Sprintf("已达最大轮数 %d，任务暂停。可以继续追加要求，或新建对话重新开始", maxRounds))
			return
		}
		s.mu.Lock()
		s.round = round
		s.mu.Unlock()
		a.emitAIState(s, "running", fmt.Sprintf("第 %d/%d 轮", round, maxRounds))

		// 1) 把最新终端内容补给模型
		ctx := a.buildContext(ts, "sinceLast", settings.ContextLines, "")
		userMsg := "【终端输出】\n"
		if ctx.Text == "" {
			userMsg += "（本轮没有新输出）"
		} else {
			userMsg += ctx.Text
		}
		s.pushHistory(aiclient.Message{Role: "user", Content: userMsg})
		s.compressHistory()

		// 2) 问模型
		s.mu.Lock()
		hist := make([]aiclient.Message, 0, len(s.history)+1)
		hist = append(hist, aiclient.Message{
			Role:    "system",
			Content: aiTakeoverSystemPrompt(s.serverName, serverHostOf(a, s.serverID)),
		})
		hist = append(hist, s.history...)
		s.mu.Unlock()

		reply, err := client.Chat(hist)
		if err != nil {
			a.emitAIState(s, "error", "调用模型失败："+err.Error())
			return
		}
		s.pushHistory(aiclient.Message{Role: "assistant", Content: reply.Text})

		cmd, thought, finish, summary := parseAICommandReply(reply.Text)
		if finish || strings.TrimSpace(cmd) == "" {
			msg := summary
			if msg == "" {
				msg = thought
			}
			if msg == "" {
				msg = "AI 认为任务已完成"
			}
			a.emitAIState(s, "done", msg)
			return
		}

		// 3) 执行这一条
		level, reason := riskOf(cmd)
		a.emitAIStep(s, models.AIStep{
			Round: round, Thought: thought, Command: cmd, Risk: level, State: "running",
		})
		res := a.aiExecStep(s, cmd)

		state := "done"
		switch res.Status {
		case "done":
			if res.ExitCode != 0 {
				state = "failed"
			}
		case "cancelled":
			a.emitAIStep(s, models.AIStep{Round: round, Command: cmd, Risk: level, State: "failed", Result: &res})
			a.emitAIState(s, "stopped", "已停止")
			return
		default:
			state = "blocked"
		}
		a.emitAIStep(s, models.AIStep{
			Round: round, Command: cmd, Risk: level, State: state, Result: &res,
		})
		if reason != "" && level == "high" {
			// 高风险命令照跑（用户已确认全权模式），但把风险提示带进下一轮上下文
			s.pushHistory(aiclient.Message{Role: "user", Content: "【提醒】上一条命令被判定为高风险：" + reason})
		}
	}
}

// looksLikePasswordPrompt 判断命中项是不是"要密码"（决定给用户哪句引导）
func looksLikePasswordPrompt(pat string) bool {
	p := strings.ToLower(pat)
	for _, k := range []string{"password", "passwd", "密码", "sudo"} {
		if strings.Contains(p, k) {
			return true
		}
	}
	return false
}

// serverHostOf 取服务器主机名（提示词里用）
func serverHostOf(a *App, serverID string) string {
	if a.store == nil {
		return ""
	}
	if sv, err := a.store.GetServerByID(serverID); err == nil && sv != nil {
		return sv.Host
	}
	return ""
}

// probeShell 探测当前 shell 是否支持我们依赖的 POSIX 语法。
// 不支持就明确拒绝接管模式——硬上只会把终端搅乱。
func (a *App) probeShell(s *aiSession) string {
	ts := a.getTerminal(s.termID)
	if ts == nil {
		return "终端会话不存在（可能已关闭）"
	}
	stepID := randomStepID()
	inject, key := buildStepInject(stepID, `printf '%s' "$0"`)
	from := ts.tail.end()
	if err := a.TerminalWrite(s.termID, inject); err != nil {
		return "无法写入终端：" + err.Error()
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, _, _, _ := ts.tail.snapshot(from)
		rawText := string(data)
		// 同样在原始流里找标记（清洗会丢掉被 \r 擦掉的标记行）
		if idx, _, ok := findMarker(rawText, key); ok {
			out := strings.ToLower(strings.TrimSpace(stripEcho(ansi.Strip(rawText[:idx]), `printf '%s' "$0"`)))
			if out == "" {
				return "" // 空输出也当通过：多数 shell 会返回 sh/bash
			}
			for _, bad := range []string{"fish", "csh", "tcsh"} {
				if strings.Contains(out, bad) {
					return "当前 shell 是 " + out + "，不支持 AI 接管所需的 POSIX 语法；" +
						"请改用 bash/zsh/sh 后再试（建议模式下不受影响）"
				}
			}
			return ""
		}
		time.Sleep(aiPollInterval)
	}
	return "无法确认当前 shell（探测超时）。可以直接使用建议模式，或换一个 bash/zsh 终端再试"
}

// ---- 模型回复解析 ----

// parseAICommandReply 解析接管模式的回复：必须是 {"thought","command","finish","summary"}。
// 三级兜底：直接解析 → 去代码围栏 → 提取第一个平衡的 {...}；都失败就当成纯文本、不执行任何命令。
func parseAICommandReply(raw string) (command, thought string, finish bool, summary string) {
	type shape struct {
		Thought  string `json:"thought"`
		Command  string `json:"command"`
		Commands []struct {
			Command string `json:"command"`
		} `json:"commands"`
		Finish  bool   `json:"finish"`
		Summary string `json:"summary"`
		Text    string `json:"text"`
	}
	var v shape
	ok := false
	for _, cand := range jsonCandidates(raw) {
		if err := json.Unmarshal([]byte(cand), &v); err == nil {
			ok = true
			break
		}
	}
	if !ok {
		// 解析不出来就不要猜命令：把原文当说明交给用户
		return "", strings.TrimSpace(raw), true, "模型没有按约定给出命令，已停止（避免执行错误的命令）"
	}
	cmd := strings.TrimSpace(v.Command)
	if cmd == "" && len(v.Commands) > 0 {
		parts := make([]string, 0, len(v.Commands))
		for _, c := range v.Commands {
			if s := strings.TrimSpace(c.Command); s != "" {
				parts = append(parts, s)
			}
		}
		cmd = strings.Join(parts, "\n")
	}
	thought = strings.TrimSpace(v.Thought)
	if thought == "" {
		thought = strings.TrimSpace(v.Text)
	}
	return cmd, thought, v.Finish, strings.TrimSpace(v.Summary)
}

// parseAISuggestReply 解析建议模式的回复：{"text": "...", "commands":[{command,reason}]}
// 解析失败时退化为"纯文本 + 从 ``` 代码块里提取命令"。
func parseAISuggestReply(raw string) (text string, cmds []models.AICommand) {
	type shape struct {
		Text     string `json:"text"`
		Commands []struct {
			Command string `json:"command"`
			Reason  string `json:"reason"`
		} `json:"commands"`
	}
	var v shape
	for _, cand := range jsonCandidates(raw) {
		if err := json.Unmarshal([]byte(cand), &v); err == nil {
			if strings.TrimSpace(v.Text) != "" || len(v.Commands) > 0 {
				text = strings.TrimSpace(v.Text)
				for _, c := range v.Commands {
					if s := strings.TrimSpace(c.Command); s != "" {
						cmds = append(cmds, models.AICommand{Command: s, Reason: strings.TrimSpace(c.Reason)})
					}
				}
				return text, cmds
			}
		}
	}
	// 兜底：原文当回答，把 ``` 代码块里的内容当命令
	text = strings.TrimSpace(raw)
	for _, block := range fencedBlocks(raw) {
		for _, line := range strings.Split(block, "\n") {
			l := strings.TrimSpace(line)
			l = strings.TrimPrefix(l, "$ ")
			if l == "" || strings.HasPrefix(l, "#") {
				continue
			}
			cmds = append(cmds, models.AICommand{Command: l})
			if len(cmds) >= 3 {
				return text, cmds
			}
		}
	}
	return text, cmds
}

// jsonCandidates 给出几种可能的 JSON 载荷：原文、去围栏后、第一个平衡的花括号对象
func jsonCandidates(raw string) []string {
	out := []string{}
	t := strings.TrimSpace(raw)
	if t != "" {
		out = append(out, t)
	}
	if un := unfence(t); un != t && un != "" {
		out = append(out, un)
	}
	if obj, ok := firstJSONObject(t); ok {
		out = append(out, obj)
	}
	return out
}

// unfence 去掉 ```json ... ``` 围栏
func unfence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return t
	}
	if i := strings.Index(t, "\n"); i >= 0 {
		t = t[i+1:]
	}
	if i := strings.LastIndex(t, "```"); i >= 0 {
		t = t[:i]
	}
	return strings.TrimSpace(t)
}

// firstJSONObject 提取第一个花括号平衡的 JSON 对象（忽略字符串里的括号）
func firstJSONObject(s string) (string, bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return "", false
	}
	depth := 0
	inStr := false
	esc := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inStr {
			if esc {
				esc = false
				continue
			}
			if c == '\\' {
				esc = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[start : i+1]), true
			}
		}
	}
	return "", false
}

// fencedBlocks 取出所有 ``` 代码块的内容
func fencedBlocks(s string) []string {
	var out []string
	rest := s
	for {
		i := strings.Index(rest, "```")
		if i < 0 {
			return out
		}
		rest = rest[i+3:]
		if j := strings.Index(rest, "\n"); j >= 0 {
			rest = rest[j+1:]
		}
		k := strings.Index(rest, "```")
		if k < 0 {
			out = append(out, rest)
			return out
		}
		out = append(out, rest[:k])
		rest = rest[k+3:]
	}
}

// ---- 命令风险判定 ----

// highRiskPatterns 破坏性操作：全权模式下照跑（用户已明确确认），但要在界面上醒目标注
var highRiskPatterns = []string{
	"rm -rf /", "rm -fr /", "mkfs", "dd if=", "dd of=/dev/", ":(){", "shutdown", "reboot",
	"init 0", "halt", "poweroff", "chmod -R 777 /", "> /etc/", "mv /* ", "iptables -f",
	"kill -9 1", "> /dev/sd", "wipefs", "fdisk", "parted",
}

// mediumRiskPatterns 有副作用：会删/改/停/装东西
var mediumRiskPatterns = []string{
	"rm ", "rm -", "systemctl stop", "systemctl disable", "systemctl restart", "service ",
	"apt remove", "apt purge", "apt-get remove", "apt-get purge", "yum remove", "dnf remove",
	"docker rm", "docker rmi", "docker stop", "docker prune", "system prune", "truncate", "sed -i", "chmod",
	"chown", "useradd", "userdel", "passwd", "kill ", "pkill", "killall",
	"curl | sh", "curl | bash", "wget | sh", "wget | bash", "pip install", "npm install -g",
	"apt install", "apt-get install", "yum install", "dnf install", "systemctl start",
	"systemctl enable", "mv ", "cp -r", ">", ">>", "tar -x", "unzip",
}

// riskOf 判定命令风险等级：low | medium | high（只用于展示，不阻断——全权模式是真全权）
func riskOf(cmd string) (string, string) {
	low := strings.ToLower(strings.TrimSpace(cmd))
	if low == "" {
		return "low", ""
	}
	for _, p := range highRiskPatterns {
		if strings.Contains(low, p) {
			return "high", "该命令包含破坏性操作（" + strings.TrimSpace(p) + "），可能导致数据或系统不可用"
		}
	}
	for _, p := range mediumRiskPatterns {
		if strings.Contains(low, p) {
			return "medium", "该命令会修改系统状态（" + strings.TrimSpace(p) + "），请留意影响"
		}
	}
	return "low", ""
}

// stepCounter 随机数不可用时的兜底自增（保证连续两次调用不会拿到同一个 id）
var stepCounter uint64

// randomStepID 8 位十六进制标记 id。
//
// 必须真正随机：这个 id 用来在同一段终端输出里认领"自己那条命令的结束标记"，
// 两次相同就会互相误命中。早先的实现是"时间戳 + 线性同余 + 右移取位"，
// 右移把低位丢掉了，同一秒内调用会得到一模一样的 id——所以改用 crypto/rand。
func randomStepID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err == nil {
		return hex.EncodeToString(b)
	}
	n := atomic.AddUint64(&stepCounter, 1)
	return fmt.Sprintf("%08x", uint32(time.Now().UnixNano())^uint32(n*2654435761))
}
