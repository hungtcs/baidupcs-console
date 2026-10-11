package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hungtcs/baidupcs-console/internal/config"
	"github.com/hungtcs/baidupcs-console/internal/session"
)

// newTestServer 构造一个不依赖真实 BaiduPCS-Go 的处理器。
// 只要不创建任务，就不会有进程被启动。
func newTestServer(t *testing.T) http.Handler {
	t.Helper()

	cfg := config.Config{
		CLIPath:           "BaiduPCS-Go",
		SaveRoot:          t.TempDir(),
		DataDir:           t.TempDir(),
		Mode:              "locate",
		OutputBufferBytes: 1 << 20,
	}

	web := fstest.MapFS{
		"index.html":    &fstest.MapFile{Data: []byte("<html>baidupcs-console</html>")},
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('app')")},
	}

	return New(cfg, newManager(t, cfg), web, "test")
}

// newManager 造一个用临时目录做数据目录的管理器。
func newManager(t *testing.T, cfg config.Config) *session.Manager {
	t.Helper()
	mgr, err := session.NewManager(cfg)
	if err != nil {
		t.Fatalf("创建任务管理器失败: %v", err)
	}
	return mgr
}

func do(t *testing.T, h http.Handler, method, target string, body string) *httptest.ResponseRecorder {
	t.Helper()

	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}

	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	rec := do(t, newTestServer(t), http.MethodGet, "/api/health", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码应为 200，实际 %d，响应体 %s", rec.Code, rec.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("响应不是合法 JSON: %v", err)
	}
	if got["ok"] != true {
		t.Errorf("ok 应为 true，实际 %v", got["ok"])
	}
	if got["cliPath"] != "BaiduPCS-Go" {
		t.Errorf("cliPath 不对: %v", got["cliPath"])
	}
}

func TestListTasksEmpty(t *testing.T) {
	rec := do(t, newTestServer(t), http.MethodGet, "/api/tasks", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码应为 200，实际 %d", rec.Code)
	}

	// 空列表必须序列化成 []，不能是 null，否则前端要额外判空。
	if !strings.Contains(rec.Body.String(), `"tasks":[]`) {
		t.Errorf("空任务列表应序列化为 []，实际 %s", rec.Body.String())
	}
}

func TestGetMissingTaskReturns404(t *testing.T) {
	rec := do(t, newTestServer(t), http.MethodGet, "/api/tasks/t_nope", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码应为 404，实际 %d", rec.Code)
	}

	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("错误响应不是合法 JSON: %v", err)
	}
	if got["error"] == "" {
		t.Errorf("错误响应应带 error 字段，实际 %s", rec.Body.String())
	}
}

func TestCreateTaskRejectsEmptyPath(t *testing.T) {
	// 空路径必须在调 CLI 之前就被挡掉，这样这个用例不会碰到真实网盘。
	rec := do(t, newTestServer(t), http.MethodPost, "/api/tasks", `{"remotePath":""}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("状态码应为 400，实际 %d，响应体 %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "网盘路径不能为空") {
		t.Errorf("错误信息不对: %s", rec.Body.String())
	}
}

func TestStaticAndSPAFallback(t *testing.T) {
	h := newTestServer(t)

	cases := []struct {
		name     string
		target   string
		wantBody string
	}{
		{"根路径回落到 index.html", "/", "baidupcs-console"},
		{"静态资源按原样返回", "/assets/app.js", "console.log"},
		{"未知路径回落到 index.html", "/some/spa/route", "baidupcs-console"},
		{"嵌套的未知路径也回落", "/a/b/c", "baidupcs-console"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, c.target, "")

			if rec.Code != http.StatusOK {
				t.Fatalf("状态码应为 200，实际 %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), c.wantBody) {
				t.Errorf("响应体应包含 %q，实际 %q", c.wantBody, rec.Body.String())
			}
		})
	}
}

func TestSPAFallbackWhenFrontendMissing(t *testing.T) {
	cfg := config.Config{
		CLIPath:           "BaiduPCS-Go",
		SaveRoot:          t.TempDir(),
		DataDir:           t.TempDir(),
		OutputBufferBytes: 1 << 20,
	}
	// 模拟前端还没构建过：dist 里只有 .gitkeep。
	web := fstest.MapFS{".gitkeep": &fstest.MapFile{}}

	h := New(cfg, newManager(t, cfg), web, "test")
	rec := do(t, h, http.MethodGet, "/", "")

	if rec.Code != http.StatusNotFound {
		t.Fatalf("状态码应为 404，实际 %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "pnpm build") {
		t.Errorf("应提示去构建前端，实际 %q", rec.Body.String())
	}
}

// 路径里带 .. 时不能读到 dist 之外的东西。
func TestSPAHandlerRejectsTraversal(t *testing.T) {
	h := newTestServer(t)

	for _, target := range []string{"/../go.mod", "/assets/../../go.mod", "/..%2Fgo.mod"} {
		rec := do(t, h, http.MethodGet, target, "")
		if strings.Contains(rec.Body.String(), "module github.com/hungtcs") {
			t.Errorf("%s 读到了仓库里的文件，存在目录穿越", target)
		}
	}
}
