package collect

import (
	"os"
	"path/filepath"
	"testing"
)

// 真实 nvidia-smi 输出（ai.lan，driver 580.159.03）：
// CSV 逗号后带空格；新格式首字段为 gpu_uuid，同一 PID 每卡一行（显存为该卡占用）。
const appsSectionSample = "GPU-aef2672d-3f20-3578-c87c-7c82fbe72e0d, 46153, /home/wx/llama-cpp-turboquant-new/build/bin/llama-server, 20002\nGPU-9807f1be-1c4e-d677-7913-129b6c507540, 46153, /home/wx/llama-cpp-turboquant-new/build/bin/llama-server, 20038"

func TestParseGPUApps(t *testing.T) {
	byUUID := parseGPUApps(appsSectionSample)
	if len(byUUID) != 2 {
		t.Fatalf("期望 2 张卡，实际 %d: %+v", len(byUUID), byUUID)
	}
	a0 := byUUID["GPU-aef2672d-3f20-3578-c87c-7c82fbe72e0d"]
	a1 := byUUID["GPU-9807f1be-1c4e-d677-7913-129b6c507540"]
	if len(a0) != 1 || a0[0].PID != 46153 || a0[0].Name != "llama-server" || a0[0].MemMB != 20002 {
		t.Fatalf("GPU0 进程信息错误: %+v", a0)
	}
	if len(a1) != 1 || a1[0].PID != 46153 || a1[0].Name != "llama-server" || a1[0].MemMB != 20038 {
		t.Fatalf("GPU1 进程信息错误: %+v", a1)
	}
}

func TestParseGPUAppsSameCardMultiContext(t *testing.T) {
	// 同卡同 PID 多行（多 CUDA 上下文）→ 按 PID 求和
	section := "GPU-aaa, 100, /opt/llama-server, 1024\nGPU-aaa, 100, /opt/llama-server, 2048\nGPU-bbb, 200, /usr/bin/python3, 512"
	byUUID := parseGPUApps(section)
	if len(byUUID) != 2 {
		t.Fatalf("期望 2 张卡，实际 %d: %+v", len(byUUID), byUUID)
	}
	a := byUUID["GPU-aaa"]
	if len(a) != 1 || a[0].PID != 100 || a[0].MemMB != 1024+2048 {
		t.Fatalf("同卡求和错误: %+v", a)
	}
	b := byUUID["GPU-bbb"]
	if len(b) != 1 || b[0].PID != 200 || b[0].Name != "python3" || b[0].MemMB != 512 {
		t.Fatalf("GPU-bbb 错误: %+v", b)
	}
}

func TestParseGPUAppsLegacyFormat(t *testing.T) {
	// 旧驱动无 gpu_uuid（3 字段）：uuid 为空，调用方回退挂第一张卡；显存按 PID 求和
	section := "1693, /opt/llama-server, 17920\n1693, /opt/llama-server, 18456"
	byUUID := parseGPUApps(section)
	apps, ok := byUUID[""]
	if !ok || len(apps) != 1 {
		t.Fatalf("期望空 uuid 下 1 个进程，实际: %+v", byUUID)
	}
	if apps[0].PID != 1693 || apps[0].Name != "llama-server" || apps[0].MemMB != 17920+18456 {
		t.Fatalf("旧格式解析错误: %+v", apps[0])
	}
}

func TestParseGPUAppsEmpty(t *testing.T) {
	if byUUID := parseGPUApps(""); len(byUUID) != 0 {
		t.Fatalf("空输入应返回空 map，实际 %+v", byUUID)
	}
	if byUUID := parseGPUApps("   \n\n"); len(byUUID) != 0 {
		t.Fatalf("空白输入应返回空 map，实际 %+v", byUUID)
	}
}

// ---------- findPID ----------

// writeProc 在假 proc 目录下建一个进程条目（comm 可能为空串=内核线程无 cmdline）。
func writeProc(t *testing.T, dir, pid, comm, cmdline string) {
	t.Helper()
	d := filepath.Join(dir, pid)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "comm"), []byte(comm), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "cmdline"), []byte(cmdline), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindPIDCommExact(t *testing.T) {
	dir := t.TempDir()
	writeProc(t, dir, "123", "llama-server\n", "/usr/bin/llama-server\x00--model\x00x")
	writeProc(t, dir, "456", "bash\n", "/bin/bash")
	if got := findPIDIn("llama-server", dir); got != 123 {
		t.Fatalf("期望 123，实际 %d", got)
	}
}

func TestFindPIDCmdlineFallback(t *testing.T) {
	// comm 被内核截断到 15 字符：全名只能靠 cmdline argv[0] basename 匹配
	dir := t.TempDir()
	writeProc(t, dir, "123", "llama-cpp-turbo\n", "/home/wx/build/bin/llama-cpp-turboquant\x00--model\x00x")
	if got := findPIDIn("llama-cpp-turboquant", dir); got != 123 {
		t.Fatalf("全名 cmdline 回退期望 123，实际 %d", got)
	}
	// 截断名仍走 comm 精确匹配（pass 1 优先）
	if got := findPIDIn("llama-cpp-turbo", dir); got != 123 {
		t.Fatalf("截断名 comm 匹配期望 123，实际 %d", got)
	}
}

func TestFindPIDNotFound(t *testing.T) {
	dir := t.TempDir()
	writeProc(t, dir, "1", "kthreadd\n", "") // 内核线程：cmdline 为空
	writeProc(t, dir, "2", "bash\n", "/bin/bash")
	if got := findPIDIn("llama-server", dir); got != 0 {
		t.Fatalf("期望 0，实际 %d", got)
	}
}

// ---------- parseCmdline ----------

func TestParseCmdlineConsecutiveFlags(t *testing.T) {
	// 连续「flag+值」对：旧版 off-by-one 会跳过值后面的下一个 flag
	cmd := "/tmp/llama-server 25 --model /tmp/fake-model.gguf --mmproj /tmp/fake-mmproj.bin"
	f := parseCmdline(cmd)
	if f["model"] != "/tmp/fake-model.gguf" {
		t.Fatalf("model 解析错误: %+v", f)
	}
	if f["mmproj"] != "/tmp/fake-mmproj.bin" {
		t.Fatalf("mmproj 解析错误: %+v", f)
	}
}

func TestParseCmdlineBoolFlagNoSkip(t *testing.T) {
	// 布尔开关不取值：后面的 flag 不能被跳过
	cmd := "llama-server --kv-offload --ctx-size 8192"
	f := parseCmdline(cmd)
	if f["kv_offload"] != "true" {
		t.Fatalf("kv_offload 解析错误: %+v", f)
	}
	if f["ctx_size"] != "8192" {
		t.Fatalf("ctx_size 解析错误: %+v", f)
	}
}
