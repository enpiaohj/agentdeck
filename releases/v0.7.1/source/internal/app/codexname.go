package app

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/enpiaohj/agentdeck/internal/proc"
	"github.com/enpiaohj/agentdeck/internal/sessions"
)

// codexNameTimeout 是一次原生改名从起进程到收答复的总预算。codex app-server
// 要冷启动一个 node 进程,给宽一点,但必须有上限——它挂在「继续会话」按钮上。
// 变量而非常量,便于测试用短的预算走超时路径。
var codexNameTimeout = 20 * time.Second

// codexClientVersion 只用于 app-server 的 initialize 握手,不参与任何校验。
const codexClientVersion = "agentdeck"

// codexThreadIDRe 校验线程 id:它来自会话文件(不可信输入),会被写进
// JSON-RPC 参数。与 sessions 包的 safeID 同规则。
var codexThreadIDRe = regexp.MustCompile(`^[0-9A-Za-z_-]+$`)

// setCodexThreadName 用 codex 自带的 app-server 把名字写进 Codex 的会话索引
// (session_index.jsonl 的 thread_name),codex 恢复后 CLI 内部显示的就是这个
// 名字。这正是 AgentDeck 读的那份索引(见 sessions.CodexTitles)。
//
// 为什么只能这么做:codex 没有 rename 子命令(只有 resume/archive/delete/
// unarchive),thread/name/set 是它自己提供的唯一写入口,且在 app-server 里
// 标着 [experimental]。写入方是 codex 自己,AgentDeck 不直接改 ~/.codex 下的
// 任何文件——默认只读的边界没有破。
func setCodexThreadName(threadID, name string) error {
	if !codexThreadIDRe.MatchString(threadID) || name == "" {
		return fmt.Errorf("Codex 会话标识或名称不合法")
	}
	path, err := proc.LookPath("codex")
	if err != nil {
		return fmt.Errorf("找不到 codex 可执行文件：%w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), codexNameTimeout)
	defer cancel()
	cmd := proc.CommandContext(ctx, path, "app-server", "--stdio")
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 codex app-server 失败：%w", err)
	}
	rpc := &codexRPC{enc: json.NewEncoder(stdin), in: bufio.NewScanner(stdout)}
	rpc.in.Buffer(make([]byte, 0, 64<<10), 4<<20)

	// 读答复与收尾都在这一条 goroutine 里顺序做,管道不再被第二处碰:
	// 先读答复,再关 stdin——app-server 读到 EOF 会自己退出(实测 2 秒内),
	// 真身 node 随之消失,卡在 Scan 上的读取也就等到了 EOF。通道有缓冲,
	// 超时后没人接也不会把这条 goroutine 吊住。
	done := make(chan error, 1)
	go func() {
		err := rpc.exchange(threadID, name)
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait() // WaitDelay 兜底,进程树赖着不散也不会卡死
		done <- err
	}()

	// 等待必须在这里兜底,不能只靠 ctx:proc.CommandContext 的 ctx 结束只杀
	// **直接子进程**,而 Windows 上 codex 是 .cmd、真身 node 是孙进程,它会
	// 一直抱着我们这头的 stdout(见 proc.CommandContext 的注释与上游 #123),
	// 卡在 Scan 上的读取等不到 EOF。实测:给它 1 秒的 ctx,6 秒后读取仍未
	// 返回。少了这一层,「继续会话」会永久停在 busy,每点一次还多一个 node
	// 进程。
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// 推一把让上面那条 goroutine 能收尾(关 stdin 后 node 会退),但不等
		// 它:这里必须返回。
		stdin.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return fmt.Errorf("codex app-server 在 %s 内没有答复，原生名称可能未同步", codexNameTimeout)
	}
}

// codexRPC 是 app-server 的极简 JSON-RPC 客户端:只发请求、按 id 收答复,
// 中间穿插的通知(无 id)直接跳过。
type codexRPC struct {
	enc *json.Encoder
	in  *bufio.Scanner
}

// exchange 是一次完整的握手加改名。
func (c *codexRPC) exchange(threadID, name string) error {
	if err := c.call(1, "initialize", map[string]any{
		"clientInfo": map[string]any{"name": "agentdeck", "version": codexClientVersion},
	}); err != nil {
		return err
	}
	return c.call(2, "thread/name/set", map[string]any{"threadId": threadID, "name": name})
}

func (c *codexRPC) call(id int, method string, params any) error {
	if err := c.enc.Encode(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return fmt.Errorf("向 codex app-server 发送 %s 失败：%w", method, err)
	}
	for c.in.Scan() {
		var msg struct {
			ID    *int `json:"id"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(c.in.Bytes(), &msg) != nil || msg.ID == nil || *msg.ID != id {
			continue // 通知,或别的请求的答复
		}
		if msg.Error != nil {
			return fmt.Errorf("codex %s 失败：%s", method, msg.Error.Message)
		}
		return nil
	}
	if err := c.in.Err(); err != nil {
		return fmt.Errorf("codex %s 无答复：%w", method, err)
	}
	return fmt.Errorf("codex %s 无答复：app-server 提前退出", method)
}

// needsCodexName 判断恢复前是否要为该会话写一次 Codex 原生名。
//
// 只对**本机原生**的 codex 会话说得通:WSL 的 codex 在另一侧;网关与导入来源
// 的 ID 未必是本地线程,拿它去改本地记录是猜(手动删除那条路径有同样的来源
// 限制,见 sessions.PrepareNativeDelete);保留会话没有可恢复的命令。没有本地
// 标题时也没什么可写的。
func needsCodexName(sess sessions.Session, title string) bool {
	return sess.Agent == "codex" && sess.WSL == "" && !sess.Gateway && !sess.Imported &&
		!sess.ReadOnly && !sess.Retained && title != ""
}
