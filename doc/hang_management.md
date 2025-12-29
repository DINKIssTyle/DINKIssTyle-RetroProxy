# Hang Management Overview

## 개요

RetroProxy는 헤드리스 브라우저(Rod)를 사용하여 웹 페이지를 렌더링합니다. 페이지 로딩 중 hang(무한 대기)이 발생할 수 있으므로, 다양한 레벨에서 타임아웃과 취소 메커니즘을 구현하고 있습니다.

---

## 타임아웃 구조

### 1. HTTP 서버 레벨 (`server.go`)

| 항목 | 값 | 위치 |
|------|-----|------|
| `ReadTimeout` | 30초 | `server.Start()` |
| `WriteTimeout` | 60초 | `server.Start()` |
| Graceful Shutdown | 5초 | `server.Stop()` |
| HTTP Client (URL fetch) | 30초 | `rewriteLinksForProxy()` |
| TCP Dial (CONNECT) | 10초 | `handleConnect()` |

### 2. 렌더러 레벨 (`renderer.go`)

| 메서드 | 타임아웃 | 용도 |
|--------|----------|------|
| `RenderPage` | 30초 | 기본 HTML 추출 |
| `RenderPageFull` | 30초 | 전체 HTML 추출 |
| `RenderPageWithLayout` | 30초 | 레이아웃 요소 추출 (3.2new) |
| `RenderPageWithScreenshot` | 30초 | 스크린샷 + HTML |
| `captureLogic` | 60초 | 이미지 맵 모드 |

### 3. 렌더러 풀 레벨 (`renderer_pool.go`)

| 항목 | 값 | 설명 |
|------|-----|------|
| Pool acquire timeout | 60초 | 렌더러 획득 대기 |
| `ErrRendererBusy` | - | 모든 렌더러 사용 중 에러 |

---

## Context 기반 취소

모든 렌더링 함수는 `context.Context`를 받아 요청 취소를 지원합니다:

```go
// renderer.go
page = page.Context(ctx)  // Rod 페이지에 컨텍스트 연결

// renderer_pool.go  
func (p *RendererPool) acquire(ctx context.Context, timeout time.Duration) (*Renderer, error) {
    select {
    case renderer := <-p.renderers:
        return renderer, nil
    case <-ctx.Done():      // 요청 취소 시
        return nil, ctx.Err()
    case <-time.After(timeout):  // 타임아웃 시
        return nil, ErrRendererBusy
    }
}
```

---

## 사용자 이탈 시 처리

사용자가 페이지를 떠나면 `r.Context()`가 취소되어:
1. Rod 페이지 로딩 즉시 중단
2. 렌더러 풀에 렌더러 반환
3. 리소스 해제

---

## 에러 처리 (`server.go`)

```go
if errors.Is(err, ErrRendererBusy) || errors.Is(err, context.Canceled) {
    s.serveRetryPage(w, targetURL, "서버가 바쁩니다")
    return
}
```

- `ErrRendererBusy`: Retry 페이지 표시
- `context.Canceled`: 조용히 종료 (사용자가 떠남)

---

## 관련 파일

| 파일 | 역할 |
|------|------|
| `proxy/server.go` | HTTP 타임아웃, 에러 핸들링 |
| `proxy/renderer.go` | Rod 페이지 타임아웃, 컨텍스트 |
| `proxy/renderer_pool.go` | 풀 획득 타임아웃, Busy 에러 |
| `proxy/imageconv.go` | 이미지 fetch HTTP 타임아웃 |
