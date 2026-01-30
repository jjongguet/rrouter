# rrouter 완전 가이드

> Claude Code CLI를 위한 경량 라우팅 프록시 - 포괄적 기술 문서

---

## 목차

1. [프로젝트 개요](#1-프로젝트-개요)
2. [아키텍처](#2-아키텍처)
3. [파일 구조](#3-파일-구조)
4. [핵심 컴포넌트 상세](#4-핵심-컴포넌트-상세)
5. [설치 및 설정](#5-설치-및-설정)
6. [사용법](#6-사용법)
7. [세 가지 라우팅 모드 상세](#7-세-가지-라우팅-모드-상세)
8. [자동 라우팅 (Auto Mode)](#8-자동-라우팅-auto-mode)
9. [설정 (Configuration)](#9-설정-configuration)
10. [커스터마이징](#10-커스터마이징)
11. [트러블슈팅](#11-트러블슈팅)
12. [개발 가이드](#12-개발-가이드)

---

## 1. 프로젝트 개요

### 1.1 목적

rrouter는 Claude Code CLI와 백엔드 API 사이에서 동작하는 **경량 라우팅 프록시**입니다. 주요 특징:

- **무재시작 모드 전환**: 프록시/CLI 재시작 없이 라우팅 모드를 즉시 변경
- **자동 장애 감지**: Antigravity 실패 시 자동으로 Claude로 폴백
- **fsnotify 기반 설정 감시**: 파일 시스템 변경 즉시 반영 (I/O 오버헤드 없음)
- **글로브 패턴 매칭**: 모델명 매핑에서 와일드카드 지원

### 1.2 핵심 기능

| 기능 | 설명 |
|------|------|
| **세 가지 모드** | antigravity, claude, auto |
| **모델명 변환** | 요청의 모델명을 자동으로 변환 |
| **자동 장애 복구** | Antigravity 실패 → Claude 자동 전환 |
| **설정 감시** | fsnotify로 config 파일 변경 감지 및 즉시 리로드 |
| **헬스 엔드포인트** | 프록시 상태 및 auto mode 정보 제공 |

### 1.3 기술 스택

| 구성요소 | 기술 |
|----------|------|
| 프록시 서버 | Go 1.22+ (표준 라이브러리 + fsnotify) |
| CLI 도구 | Bash |
| 파일 감시 | fsnotify (macOS inode 교체 대응) |
| 서비스 관리 | Linux: systemd, macOS: launchd |

---

## 2. 아키텍처

### 2.1 전체 흐름도

```
┌─────────────────────────────────────────────────────────────┐
│                     Claude Code CLI                          │
│               (ANTHROPIC_BASE_URL=localhost:8316)           │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                   rrouter serve:8316                         │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  fsnotify 파일 감시 + 메모리 캐시                       │ │
│  │  ~/.rrouter/mode (antigravity|claude|auto)             │ │
│  │  ~/.rrouter/config.json (글로브 매칭)                  │ │
│  └────────────────────────────────────────────────────────┘ │
│                            │                                 │
│  1. HTTP 요청 수신 / Receive request                         │
│  2. 모드 확인 (캐시에서) / Check mode (from cache)          │
│  3. AUTO MODE: 장애 감지 및 복구 / Auto failure recovery    │
│     - 429/5xx × 3 → Claude 전환                             │
│     - timeout × 2 → Claude 전환                             │
│     - 쿨다운: 30분 ~ 4시간 (지수 백오프)                    │
│  4. 모델명 변환 / Rewrite model name (glob match)           │
│  5. cliproxyapi로 전달 / Forward to upstream                │
│                                                              │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                      cliproxyapi:8317                        │
│                  (수정 없이 그대로 / unchanged)               │
└────────────────────────────┬────────────────────────────────┘
                             │
                             ▼
                 Antigravity / Claude OAuth
              (same upstream, model rewrite only)
```

### 2.2 요청 처리 시퀀스

```
[Claude Code CLI]
      │
      │ POST /v1/messages
      │ {"model": "claude-sonnet-4-5-20250929", ...}
      ▼
[rrouter serve]
      │
      ├─→ GetMode() : mode 읽기 (캐시) → "auto"
      │
      ├─→ resolveRouting() : 실제 타겟 결정
      │   intent="auto" → "antigravity" 또는 "claude"
      │
      ├─→ getConfig() : 모드별 매핑 설정 읽기 (캐시)
      │
      ├─→ modifyRequestBody() : 모델명 변환
      │   "claude-sonnet-4-5-20250929" → "gemini-claude-sonnet-4-5-thinking"
      │
      ├─→ proxy.ServeHTTP() : upstream으로 전달
      │
      ├─→ recordUpstreamResponse() : auto mode 상태 업데이트
      │   (실패 카운트, 쿨다운, 전환 등)
      ▼
[cliproxyapi:8317]
      │
      ▼
[Antigravity/Claude OAuth]
```

### 2.3 fsnotify 감시 메커니즘

rrouter serve는 `~/.rrouter/` 디렉토리를 **fsnotify로 감시**합니다:

- **파일 I/O 최소화**: 요청마다 디스크 읽기 없음, 메모리 캐시 사용
- **즉시 반영**: 설정 파일 변경 → 자동 리로드 (수동 재시작 불필요)
- **macOS 호환**: 디렉토리 단위 감시로 inode 교체 문제 해결

```
~/.rrouter/
    ├─ mode              (변경 감지)
    │  └─ (Write event) → readModeFile() → newMode 적용
    │
    └─ config.json       (변경 감지)
       └─ (Write event) → readConfigFile() → newConfig 적용
```

---

## 3. 파일 구조

### 3.1 프로젝트 디렉토리

```
/home/forge/project_src/ForgeLab/rrouter/
├── cmd/rrouter/
│   ├── cli.go               # CLI entry point and main
│   ├── serve.go             # Proxy server (HTTP handler)
│   ├── daemon.go            # Daemon management (start/stop/status)
│   ├── mode.go              # Mode switching commands
│   ├── config_cmd.go        # Config subcommands
│   ├── proxy_config.go      # Config loading and model mapping
│   ├── auto.go              # Auto mode state management
│   ├── watcher.go           # fsnotify watcher
│   └── help.go              # Help text
├── config.json                  # Default configuration at project root
├── services/
│   ├── rrouter.service          # Linux systemd service file
│   └── com.rrouter.plist        # macOS launchd service file
├── install.sh                   # Auto-install script
├── uninstall.sh                 # Uninstall script
├── go.mod                       # Go module definition
├── go.sum                       # Dependency hashes
├── README.md                    # Quick start guide
└── RROUTER_COMPLETE_GUIDE.md    # This document
```

### 3.2 설치 후 파일 위치

```
~/.local/bin/
└── rrouter                      # Single binary (CLI + proxy server)

~/.rrouter/
├── mode                         # 현재 모드 파일 (antigravity/claude/auto)
├── config.json                  # 모델 매핑 설정 (선택사항)
└── logs/                        # 로그 디렉토리 (macOS)

~/.config/systemd/user/          # Linux
└── rrouter.service

~/Library/LaunchAgents/          # macOS
└── com.rrouter.plist

~/.claude/settings.json          # Claude Code 설정 (env.ANTHROPIC_BASE_URL)
```

---

## 4. 핵심 컴포넌트 상세

### 4.1 메인 프록시 서버 (`cmd/rrouter/serve.go`)

#### 4.1.1 전역 변수 및 상수

```go
const modeFile = ".rrouter/mode"

var (
    requestCount  atomic.Uint64      // 요청 카운터
    appConfig     *Config            // 로드된 설정
    upstreamURL   string             // 타겟 upstream URL
    listenAddr    string             // 리스닝 주소
    configWatcher *ConfigWatcher     // fsnotify 감시자
    autoSwitch    *autoState         // auto mode 상태
)
```

#### 4.1.2 요청 처리 흐름

**proxyHandler()**는 모든 HTTP 요청을 처리합니다:

```go
func proxyHandler(proxy *httputil.ReverseProxy) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. 요청 번호 증가
        reqNum := requestCount.Add(1)

        // 2. 현재 모드 읽기 (캐시에서)
        intent := configWatcher.GetMode()

        // 3. Auto mode: 실제 타겟 결정
        target := autoSwitch.resolveRouting(intent)

        // 4. 로그 기록
        log.Printf("[Req #%d] %s %s (mode: %s)", reqNum, r.Method, r.URL.Path, target)

        // 5. 요청 body 읽기 및 모델명 변환
        bodyBytes, _ := io.ReadAll(r.Body)
        r.Body.Close()

        if len(bodyBytes) > 0 {
            modifiedBody, _ := modifyRequestBody(bodyBytes, modeConfig, target)
            r.Body = io.NopCloser(bytes.NewReader(modifiedBody))
            r.ContentLength = int64(len(modifiedBody))
        }

        // 6. Upstream으로 전달
        lrw := newLoggingResponseWriter(w)
        proxy.ServeHTTP(lrw, r)

        // 7. 응답 로그
        log.Printf("[Req #%d] Response: %d", reqNum, lrw.statusCode)

        // 8. Auto mode: 응답 기록 (다음 결정을 위해)
        if intent == "auto" {
            autoSwitch.recordUpstreamResponse(lrw.statusCode, isTimeout)
        }
    }
}
```

#### 4.1.3 헬스 엔드포인트

`GET /health` 엔드포인트는 프록시 상태를 JSON으로 반환합니다:

```json
{
  "status": "ok",
  "mode": "auto",
  "currentTarget": "claude",
  "requestCount": 156,
  "listenAddr": ":8316",
  "upstreamURL": "http://localhost:8317",
  "defaultMode": "claude",
  "autoSwitched": true,
  "autoSwitchCount": 1,
  "failureCount": 3,
  "timeoutCount": 0,
  "switchedAt": "2026-01-29T15:00:00Z",
  "cooldownRemaining": "30m0s",
  "cooldownDuration": "30m0s"
}
```

### 4.2 설정 로드 (`cmd/rrouter/proxy_config.go`)

#### 4.2.1 구조체 정의

```go
type Config struct {
    Modes       map[string]ModeConfig `json:"modes"`
    DefaultMode string                `json:"defaultMode"`
}

type ModeConfig struct {
    Mappings []ModelMapping `json:"mappings"`
}

type ModelMapping struct {
    Match   string `json:"match"`      // 글로브 패턴 (e.g., "claude-sonnet-*")
    Rewrite string `json:"rewrite"`    // 변환 대상 (e.g., "gemini-claude-sonnet-4-5-thinking")
}
```

#### 4.2.2 설정 로드 시퀀스

```go
func loadConfigWithDefaults() *Config {
    // 1. ~/.rrouter/config.json 시도
    configPath := filepath.Join(homeDir, ".rrouter", "config.json")
    cfg, err := loadConfig(configPath)

    // 2. 파일 없으면 내장 기본값 사용
    if err != nil {
        log.Printf("Error reading config.json, using embedded defaults: %v", err)
        cfg = loadEmbeddedConfig()
    }

    // 3. 레거시 파일 경고
    legacyPath := filepath.Join(homeDir, ".rrouter", "config")
    if _, err := os.Stat(legacyPath); err == nil {
        log.Printf("Found legacy ~/.rrouter/config file; using config.json instead...")
    }

    return cfg
}
```

#### 4.2.3 모델 매칭

```go
func rewriteModelWithConfig(model string, modeConfig *ModeConfig) string {
    if modeConfig == nil {
        return model
    }
    // 설정된 매핑 규칙 순서대로 확인
    for _, m := range modeConfig.Mappings {
        if matchModel(m.Match, model) {  // filepath.Match 사용
            return m.Rewrite
        }
    }
    return model  // 매칭 안 되면 passthrough
}
```

### 4.3 Auto Mode 상태 관리 (`cmd/rrouter/auto.go`)

#### 4.3.1 상태 구조체

```go
type autoState struct {
    mu              sync.Mutex

    failureCount    int           // 429/5xx 연속 실패 횟수
    timeoutCount    int           // timeout 연속 발생 횟수
    switched        bool          // 현재 Claude로 전환 중인가
    switchedAt      time.Time     // 전환 시각

    cooldownDuration time.Duration // 현재 쿨다운 기간 (30m → 4h)
    cooldownTimer   *time.Timer    // 쿨다운 타이머
    generation      uint64         // 타이머 race condition 방지

    switchCount     atomic.Int64   // 총 전환 횟수
    healthySince    time.Time      // 마지막 성공 시각
}
```

#### 4.3.2 핵심 메서드

**resolveRouting()**: 라우팅 결정

```go
func (s *autoState) resolveRouting(intent string) string {
    if intent != "auto" {
        return intent  // "antigravity" 또는 "claude"
    }

    s.mu.Lock()
    defer s.mu.Unlock()

    if s.switched {
        return "claude"
    }
    return "antigravity"
}
```

**recordUpstreamResponse()**: 응답 기록 및 상태 업데이트

```go
func (s *autoState) recordUpstreamResponse(statusCode int, isTimeout bool) {
    s.mu.Lock()
    defer s.mu.Unlock()

    // 성공: 2xx 또는 non-429 4xx
    if !isTimeout && statusCode >= 200 && statusCode < 500 && statusCode != 429 {
        s.failureCount = 0
        s.timeoutCount = 0
        s.healthySince = time.Now()
        return
    }

    // Timeout 카운트
    if isTimeout {
        s.timeoutCount++
        if s.timeoutCount >= 2 && !s.switched {
            s.triggerSwitch("timeout")  // Claude로 전환
        }
        return
    }

    // 429 또는 5xx 카운트
    if statusCode == 429 || statusCode >= 500 {
        s.failureCount++
        if s.failureCount >= 3 && !s.switched {
            s.triggerSwitch(fmt.Sprintf("HTTP %d", statusCode))  // Claude로 전환
        }
    }
}
```

**triggerSwitch()**: 전환 트리거

```go
func (s *autoState) triggerSwitch(reason string) {
    s.switched = true
    s.switchedAt = time.Now()
    s.switchCount.Add(1)
    s.failureCount = 0
    s.timeoutCount = 0

    // 쿨다운 기간 증가 (지수 백오프)
    if s.switchCount.Load() > 1 {
        s.cooldownDuration = min(s.cooldownDuration*2, maxCooldown)
    }

    log.Printf("[AUTO] SWITCHING: Antigravity -> Claude")
    log.Printf("[AUTO] Cooldown: %s", s.cooldownDuration)

    s.generation++
    s.startCooldown()  // 타이머 시작
}
```

### 4.4 파일 감시 (`cmd/rrouter/watcher.go`)

#### 4.4.1 ConfigWatcher 구조체

```go
type ConfigWatcher struct {
    mu      sync.RWMutex
    mode    string          // 캐시된 모드
    config  *Config         // 캐시된 설정
    watcher *fsnotify.Watcher
    dir     string          // ~/.rrouter/
}
```

#### 4.4.2 감시 루프

```go
func (cw *ConfigWatcher) watchLoop() {
    for {
        select {
        case event, ok := <-cw.watcher.Events:
            if !ok {
                return
            }

            basename := filepath.Base(event.Name)
            if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
                switch basename {
                case "mode":
                    // ~/.rrouter/mode 변경 감지
                    newMode := cw.readModeFile()
                    cw.mu.Lock()
                    if oldMode := cw.mode; oldMode != newMode {
                        cw.mode = newMode

                        // Auto mode에서 explicit mode로 전환 시 상태 초기화
                        if oldMode == "auto" && newMode != "auto" {
                            autoSwitch.reset()  // 쿨다운 및 카운터 초기화
                        }

                        log.Printf("[WATCHER] Mode changed: %s -> %s", oldMode, newMode)
                    }
                    cw.mu.Unlock()

                case "config.json":
                    // ~/.rrouter/config.json 변경 감지
                    if cfg := cw.readConfigFile(); cfg != nil {
                        cw.mu.Lock()
                        cw.config = cfg
                        cw.mu.Unlock()
                        log.Printf("[WATCHER] Config reloaded")
                    }
                }
            }
        }
    }
}
```

#### 4.4.3 캐시 접근

```go
// 요청마다 호출되지만 I/O 없음
func (cw *ConfigWatcher) GetMode() string {
    cw.mu.RLock()
    defer cw.mu.RUnlock()
    return cw.mode  // 캐시된 값 반환
}

func (cw *ConfigWatcher) GetConfig() *Config {
    cw.mu.RLock()
    defer cw.mu.RUnlock()
    return cw.config  // 캐시된 값 반환
}
```

### 4.5 CLI 도구 및 데몬 관리

#### 4.5.1 명령어 목록

| 명령어 | 별칭 | 동작 |
|--------|------|------|
| `rrouter serve` | - | Proxy server를 foreground로 실행 |
| `rrouter start` | - | Proxy daemon을 background로 시작 |
| `rrouter stop` | - | Proxy daemon 중지 |
| `rrouter restart` | - | Proxy daemon 재시작 |
| `rrouter status` | - | 현재 모드 및 데몬 상태 표시 |
| `rrouter antigravity` | `ag` | Antigravity 모드로 전환 |
| `rrouter claude` | `c` | Claude OAuth 모드로 전환 |
| `rrouter auto` | `a` | Auto mode (자동 장애 복구) 활성화 |
| `rrouter --check` | `check`, `health` | 헬스 체크 |
| `rrouter config` | - | 현재 config.json 보기 |
| `rrouter config edit` | - | 에디터로 config.json 수정 |
| `rrouter config reset` | - | config.json 기본값으로 리셋 |
| `rrouter config path` | - | config.json 파일 경로 표시 |
| `rrouter help` | `--help`, `-h` | 도움말 표시 |

#### 4.5.2 핵심 기능들

**데몬 관리** (`cmd/rrouter/daemon.go`):
- systemd (Linux) 또는 launchd (macOS)를 통한 백그라운드 실행
- `rrouter start/stop/restart` 명령으로 통합 관리

**모드 전환** (`cmd/rrouter/mode.go`):
- 모드 파일에 쓰기
- fsnotify 감시자가 변경 감지 → 자동 반영

---

## 5. 설치 및 설정

### 5.1 자동 설치

```bash
cd /home/forge/project_src/ForgeLab/rrouter
./install.sh
```

설치 스크립트가 수행하는 작업:

1. Go 1.22+ 확인
2. `go build -o rrouter ./cmd/rrouter` 실행
3. 바이너리를 `~/.local/bin/`에 설치
4. `~/.rrouter/` 디렉토리 및 로그 디렉토리 생성
5. 기본 모드를 `claude`로 설정
6. systemd (Linux) 또는 launchd (macOS) 서비스 등록 및 시작
7. Claude Code `~/.claude/settings.json` 설정 (env.ANTHROPIC_BASE_URL)

### 5.2 수동 설치

```bash
# 1. 빌드
cd /home/forge/project_src/ForgeLab/rrouter
go build -o rrouter ./cmd/rrouter

# 2. 바이너리 설치
mkdir -p ~/.local/bin
cp rrouter ~/.local/bin/
chmod +x ~/.local/bin/rrouter

# 3. 설정 디렉토리
mkdir -p ~/.rrouter

# 4. 기본 모드 설정
echo "claude" > ~/.rrouter/mode

# 5. 서비스 등록 (Linux)
mkdir -p ~/.config/systemd/user
cp services/rrouter.service ~/.config/systemd/user/
systemctl --user daemon-reload
systemctl --user enable --now rrouter

# 또는 macOS
mkdir -p ~/Library/LaunchAgents
cp services/com.rrouter.plist ~/Library/LaunchAgents/
launchctl load ~/Library/LaunchAgents/com.rrouter.plist

# 6. Claude Code 설정
mkdir -p ~/.claude
cat > ~/.claude/settings.json << 'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF
```

### 5.3 Claude Code 설정

편집: `~/.claude/settings.json`

**방법 1: 환경변수 지정 (권장 / Recommended)**

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
```

**방법 2: 프록시 포트 지정 (레거시 / Legacy)**

```json
{
  "anthropicProxyPort": 8316
}
```
```

---

## 6. 사용법

### 6.1 빠른 시작

```bash
# 1. Auto mode 활성화 (권장)
rrouter auto

# 2. Claude Code 사용 (재시작 필요 없음)
# 자동으로 Antigravity를 시도하고, 실패 시 Claude로 전환됨
```

### 6.2 모드 전환

```bash
# Antigravity (thinking models)
rrouter antigravity
rrouter ag

# Claude (OAuth passthrough)
rrouter claude
rrouter c

# Auto (Antigravity-first with Claude fallback)
rrouter auto
rrouter a
```

### 6.3 상태 확인

```bash
# 자세한 상태 표시
rrouter status

# 출력 예시:
# ═══════════════════════════════════════
#   rrouter Status
# ═══════════════════════════════════════
#
#   Mode:        Auto (Antigravity-first, Claude fallback)
#   Mode file:   /home/user/.rrouter/mode
#
# Auto-Switch Status:
#   Currently routing: Antigravity
#   Total switches:    1 (recovered)
#
# Proxy Status:
#   rrouter daemon:         Running
#   antigravity-proxy:      Running
#
# ═══════════════════════════════════════
```

### 6.4 헬스 체크

```bash
# CLI 도구로 확인
rrouter --check
rrouter check
rrouter health

# 직접 API 호출
curl http://localhost:8316/health | jq '.'

# 출력:
# {
#   "mode": "auto",
#   "currentTarget": "antigravity",
#   "autoSwitched": false,
#   "failureCount": 0,
#   "timeoutCount": 0,
#   "requestCount": 42
# }
```

### 6.5 설정 관리

```bash
# 현재 config.json 보기
rrouter config

# 에디터로 설정 수정
rrouter config edit

# 기본값으로 리셋
rrouter config reset

# 파일 경로 확인
rrouter config path
```

### 6.6 데몬 관리

**통합 명령어 (권장)**

```bash
# 상태 확인
rrouter status

# 시작
rrouter start

# 중지
rrouter stop

# 재시작
rrouter restart
```

**직접 서비스 관리 (고급)**

**Linux (systemd)**

```bash
# 상태 확인
systemctl --user status rrouter

# 재시작
systemctl --user restart rrouter

# 로그 실시간 보기
journalctl --user -u rrouter -f

# 비활성화
systemctl --user disable rrouter
```

**macOS (launchd)**

```bash
# 목록 확인
launchctl list | grep rrouter

# 재시작
launchctl kickstart -k gui/$(id -u)/com.rrouter

# 언로드
launchctl unload ~/Library/LaunchAgents/com.rrouter.plist

# 로드
launchctl load ~/Library/LaunchAgents/com.rrouter.plist

# 로그 확인
tail -f ~/.rrouter/logs/rrouter.log
```

---

## 7. 세 가지 라우팅 모드 상세

### 7.1 Antigravity 모드 (`antigravity`)

**목적**: Gemini Thinking 모델 사용

**동작**:
- 모든 Claude 모델명을 Gemini 모델명으로 변환
- 설정된 매핑 규칙 사용:
  - `claude-sonnet-*` → `gemini-claude-sonnet-4-5-thinking`
  - `claude-opus-*` → `gemini-claude-opus-4-5-thinking`
  - `claude-haiku-*` → `gemini-3-flash-preview`

**사용 시나리오**:
- Thinking models 활용 필요
- 추론/분석 작업에 사용 (더 높은 품질)

**전환**:

```bash
rrouter antigravity
# 또는
rrouter ag
```

### 7.2 Claude 모드 (`claude`)

**목적**: Claude OAuth 직접 사용 (passthrough)

**동작**:
- 모든 요청을 그대로 upstream으로 전달
- 모델명 변환 없음
- 빠른 응답, 저비용

**사용 시나리오**:
- Antigravity 이용 불가
- 빠른 응답 필요
- 비용 절감

**전환**:

```bash
rrouter claude
# 또는
rrouter c
```

### 7.3 Auto 모드 (`auto`) - 권장

**목적**: Antigravity 우선, 장애 시 자동 Claude 폴백

**동작**:
- 기본값: Antigravity로 라우팅
- 연속 실패 시 자동으로 Claude로 전환
- 일정 시간(쿨다운) 후 Antigravity 재시도
- 성공하면 쿨다운 리셋

**장점**:
- 항상 최적 모델 사용 시도
- 실패 시 자동 복구
- 수동 개입 불필요
- 재시작 불필요

**전환**:

```bash
rrouter auto
# 또는
rrouter a
```

---

## 8. 자동 라우팅 (Auto Mode)

### 8.1 작동 원리

```
[사용자 시작]
    │
    ▼
rrouter auto
    │
    ▼
mode 파일에 "auto" 기록
    │
    ▼
[다음 Claude Code 요청부터]
    │
    ▼
proxyHandler() → resolveRouting("auto") → "antigravity" 반환
    │
    ├─→ 요청 전달 → Antigravity 처리
    │
    ▼ (응답 결과)
    │
    ├─ HTTP 2xx (성공)
    │  └─ 카운터 리셋 → 계속 Antigravity 사용
    │
    ├─ HTTP 429 (rate limit) × 3
    │  └─ triggerSwitch() → Antigravity → Claude 전환
    │     쿨다운 시작: 30분
    │
    ├─ HTTP 5xx (server error) × 3
    │  └─ triggerSwitch() → Claude 전환
    │
    └─ Timeout × 2
       └─ triggerSwitch() → Claude 전환
```

### 8.2 탐지 규칙

| 오류 유형 | 임계값 | 추적 | 리셋 조건 |
|-----------|--------|------|-----------|
| HTTP 429 | 3회 연속 | 별도 카운터 | 2xx 또는 non-429 4xx |
| HTTP 5xx | 3회 연속 | 별도 카운터 | 2xx 또는 non-429 4xx |
| Timeout | 2회 연속 | 별도 카운터 | 모든 성공 응답 |
| HTTP 4xx (429 제외) | - | 카운트 안 함 | - |
| HTTP 2xx | - | 모든 카운터 리셋 | - |

**중요**: Timeout과 HTTP 오류는 **별도 카운터**로 추적됩니다.

### 8.3 쿨다운 및 재시도

| 시도 | 쿨다운 | 트리거 |
|------|--------|--------|
| 1차 전환 | 30분 | 초기 실패 |
| 2차 전환 | 60분 | 30분 후 재시도 시 재실패 |
| 3차 전환 | 120분 | 다시 2배 증가 |
| 4차 이상 | 240분 (최대) | 4시간 상한선 |

**쿨다운 리셋 조건**:
- **수동 전환**: `rrouter ag` 또는 `rrouter claude` → 모든 상태 초기화
- **프록시 재시작**: Antigravity부터 새로 시작
- **지속적 정상**: 현재 쿨다운의 2배 시간 동안 정상 작동 → 30분으로 리셋

### 8.4 상태 전환 다이어그램

```
                    ┌─────────────────┐
                    │   Antigravity   │
                    │   (default)     │
                    └────────┬────────┘
                             │
                  429/5xx ×3 │ timeout ×2
                             │
                             ▼
                    ┌─────────────────┐
                    │     Claude      │
                    │   (fallback)    │
                    └────────┬────────┘
                             │
                  Cooldown expires (30m ~ 240m)
                             │
                             ▼
                    ┌─────────────────┐
              ┌────▶│   Antigravity   │
              │     │     (retry)     │
              │     └────────┬────────┘
              │              │
    Success (2xx)     429/5xx ×3 again
    Reset cooldown           │
              │              ▼
              │     ┌─────────────────┐
              └─────│     Claude      │
                    │ (cooldown ×2)   │
                    └─────────────────┘
```

### 8.5 예시 시나리오

```
1. 사용자: rrouter auto
   → mode 파일에 "auto" 기록
   → 프록시가 감시자를 통해 변경 감지

2. 첫 Claude Code 요청
   → proxyHandler() 실행
   → resolveRouting("auto") → "antigravity" 반환
   → Antigravity로 라우팅

3. Antigravity 응답: HTTP 429
   → failureCount++ (1/3)

4. 두 번째 요청: HTTP 429
   → failureCount++ (2/3)

5. 세 번째 요청: HTTP 429
   → failureCount++ (3/3)
   → triggerSwitch() → switched=true, cooldownDuration=30m
   → 다음 요청부터 Claude로 라우팅
   → 로그: "[AUTO] SWITCHING: Antigravity -> Claude"

6. 30분 경과
   → cooldownTimer 발동
   → switched=false → 다시 Antigravity 시도

7. Antigravity 재시도: HTTP 429 × 3 다시 발생
   → triggerSwitch() → cooldownDuration을 2배 (60분)
   → 다시 Claude로 전환

8. 사용자: rrouter ag (수동 전환)
   → autoSwitch.reset() 호출
   → 모든 카운터/타이머 초기화
   → 즉시 Antigravity로 시도
```

### 8.6 제약사항

| 항목 | 제약 |
|------|------|
| **탐지 범위** | Pre-stream errors only (SSE 스트림 중간 오류 탐지 불가) |
| **상태 저장** | In-memory only (프록시 재시작 시 초기화) |
| **라우팅 방식** | 같은 upstream, 모델명 재작성만 (URL 전환 없음) |
| **카운터 독립성** | Timeout과 HTTP 실패는 별도 추적 |

---

## 9. 설정 (Configuration)

### 9.1 모드 파일

**위치**: `~/.rrouter/mode`

**형식**: 단순 텍스트 파일, 단일 줄

**유효한 값**:
- `antigravity` - Antigravity 모드
- `claude` - Claude 모드
- `auto` - Auto mode (자동 장애 복구)

**예시**:

```
auto
```

### 9.2 설정 파일 (`config.json`)

**위치**: `~/.rrouter/config.json`

**필수성**: 선택사항 (없으면 내장 기본값 사용)

**형식**: JSON

#### 9.2.1 예시 설정

```json
{
  "modes": {
    "antigravity": {
      "mappings": [
        {
          "match": "claude-sonnet-*",
          "rewrite": "gemini-claude-sonnet-4-5-thinking"
        },
        {
          "match": "claude-opus-*",
          "rewrite": "gemini-claude-opus-4-5-thinking"
        },
        {
          "match": "claude-haiku-*",
          "rewrite": "gemini-3-flash-preview"
        }
      ]
    },
    "claude": {
      "mappings": []
    }
  },
  "defaultMode": "claude"
}
```

#### 9.2.2 글로브 패턴 매칭

`match` 필드는 Go `filepath.Match` 패턴을 지원합니다:

| 패턴 | 매칭 대상 | 예시 |
|------|---------|------|
| `claude-sonnet-*` | 모든 Sonnet 버전 | `claude-sonnet-4-5-20250929`, `claude-sonnet-3-20240229` |
| `claude-opus-4-5-*` | Opus 4.5.x 버전 | `claude-opus-4-5-20251101` |
| `claude-haiku-*` | 모든 Haiku 버전 | `claude-haiku-4-5-20251001` |
| `*` | 모든 모델 | (모든 요청) |
| `claude-*` | 모든 Claude 모델 | (모든 Claude 모델) |

**매칭 규칙**:
- 설정된 순서대로 확인
- 첫 번째 매칭 규칙 적용
- 매칭 없으면 passthrough

#### 9.2.3 모드별 매핑 규칙

**antigravity**:
- 글로브 패턴과 재작성 규칙 적용
- `match` → `rewrite` 변환

**claude**:
- `mappings: []` (빈 배열)
- Passthrough (변환 없음)

**auto**:
- 현재 활성 타겟(antigravity/claude)의 규칙 사용
- Antigravity 중 → antigravity 규칙
- Claude 중 → passthrough

### 9.3 환경변수

**Go 바이너리에서 존중됨**:

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `RROUTER_PORT` | 8316 | 프록시 리스닝 포트 |
| `RROUTER_UPSTREAM` | http://localhost:8317 | Upstream URL (cliproxyapi) |

**설정 방법**:

```bash
# 직접 실행
RROUTER_PORT=9000 rrouter serve

# systemd 서비스 파일에서
# ~/.config/systemd/user/rrouter.service
[Service]
Environment=RROUTER_PORT=9000
Environment=RROUTER_UPSTREAM=http://localhost:9317
```

**macOS launchd 사용**:

```xml
<!-- ~/Library/LaunchAgents/com.rrouter.plist -->
<dict>
  <key>EnvironmentVariables</key>
  <dict>
    <key>RROUTER_PORT</key>
    <string>9000</string>
  </dict>
</dict>
```

---

## 10. 커스터마이징

### 10.1 새 모드 추가

**파일**: `config.json` (프로젝트 루트)

```json
{
  "modes": {
    "antigravity": { ... },
    "claude": { ... },
    "my-custom-mode": {
      "mappings": [
        {
          "match": "claude-sonnet-*",
          "rewrite": "my-custom-sonnet-model"
        },
        {
          "match": "claude-opus-*",
          "rewrite": "my-custom-opus-model"
        }
      ]
    }
  }
}
```

**CLI 업데이트**: `cmd/rrouter/mode.go` 또는 `cmd/rrouter/cli.go`

```go
// Add case in CLI routing
case "my-custom", "mc":
    cmdMyCustom()

func cmdMyCustom() {
    setMode("my-custom-mode")
    fmt.Println("Mode set to: my-custom-mode")
}
```

**빌드 및 배포**:

```bash
go build -o rrouter ./cmd/rrouter
cp rrouter ~/.local/bin/
rrouter restart
```

### 10.2 모델 매핑 커스터마이징

사용자 설정 파일에서 매핑 변경:

```bash
rrouter config edit
```

또는 직접 편집:

```bash
cat > ~/.rrouter/config.json <<'EOF'
{
  "modes": {
    "antigravity": {
      "mappings": [
        {
          "match": "claude-*",
          "rewrite": "my-custom-model"
        }
      ]
    },
    "claude": {
      "mappings": []
    }
  },
  "defaultMode": "claude"
}
EOF
```

프록시가 감시자를 통해 변경을 감지하고 자동 리로드합니다 (재시작 불필요).

### 10.3 포트 변경

**방법 1: 환경변수**

```bash
RROUTER_PORT=9000 rrouter serve
```

**방법 2: systemd 서비스**

```bash
# ~/.config/systemd/user/rrouter.service 편집
[Service]
Environment=RROUTER_PORT=9000

systemctl --user daemon-reload
systemctl --user restart rrouter
```

**Claude Code 설정도 업데이트**:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:9000"
  }
}
```

### 10.4 Upstream URL 변경

**방법 1: 환경변수**

```bash
RROUTER_UPSTREAM=http://other-proxy:8080 rrouter serve
```

**방법 2: systemd 서비스**

```ini
Environment=RROUTER_UPSTREAM=http://other-proxy:8080
```

### 10.5 기본 모드 변경

**설정 파일에서**:

```json
{
  "modes": { ... },
  "defaultMode": "antigravity"
}
```

또는 CLI에서:

```bash
# 시작할 때마다 auto 모드 활성화
rrouter auto
```

---

## 11. 트러블슈팅

### 11.1 프록시가 실행되지 않음

**진단**:

```bash
# 상태 확인
rrouter status

# 프로세스 확인
pgrep -fl rrouter

# 포트 확인
lsof -i :8316
lsof -i :8317
```

**로그 확인**:

```bash
# Linux
journalctl --user -u rrouter -f

# macOS
tail -f ~/.rrouter/logs/rrouter.log
```

**일반적인 원인**:
- 포트 충돌 (8316 또는 8317 이미 사용)
- 권한 문제
- 서비스 활성화 안 됨

**해결**:

```bash
# 포트 변경
RROUTER_PORT=9000 rrouter serve

# 또는 데몬 재시작
rrouter restart

# 직접 서비스 재시작
systemctl --user restart rrouter  # Linux
launchctl kickstart -k gui/$(id -u)/com.rrouter  # macOS
```

### 11.2 모드 전환이 안 됨

**진단**:

```bash
# 모드 파일 확인
cat ~/.rrouter/mode

# 프록시 health 확인
curl http://localhost:8316/health | jq '.'
```

**일반적인 원인**:
- 모드 파일 없음
- 프록시가 변경을 감지 못함 (감시자 오류)
- 설정 권한 문제

**해결**:

```bash
# 모드 파일 생성
echo "claude" > ~/.rrouter/mode

# 데몬 재시작
rrouter restart

# 또는 수동 실행 (로그 확인용)
rrouter serve
```

### 11.3 Auto mode가 작동하지 않음

**진단**:

```bash
# 현재 모드 확인
cat ~/.rrouter/mode

# Auto 상태 확인
curl http://localhost:8316/health | jq '.autoSwitched, .failureCount'

# 로그 확인
journalctl --user -u rrouter -f | grep AUTO
```

**일반적인 원인**:
- antigravity-proxy 실행 중이 아님
- 설정 오류
- 응답 코드 감지 오류

**해결**:

```bash
# antigravity-proxy 시작 확인
pgrep -fl antigravity-proxy

# 수동 테스트
curl -X POST http://localhost:8316/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-sonnet-4-5-20250929", "messages": [{"role": "user", "content": "test"}]}'
```

### 11.4 Claude Code 연결 안 됨

**진단**:

```bash
# 설정 확인
cat ~/.claude/settings.json

# 프록시 health 확인
curl http://localhost:8316/health

# 포트 확인
lsof -i :8316
```

**일반적인 원인**:
- 프록시 설정 오류
- Claude Code 재시작 필요
- 포트 충돌

**해결**:

```bash
# 설정 재설정
cat > ~/.claude/settings.json << 'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF

# Claude Code 재시작
# (또는 새 터미널에서 claude 명령 실행)

# 데몬 재시작
rrouter restart
```

### 11.5 모델 변환이 작동하지 않음

**진단**:

```bash
# 로그에서 변환 확인
journalctl --user -u rrouter -f | grep "Rewriting"

# 설정 확인
cat ~/.rrouter/config.json
```

**일반적인 원인**:
- 설정 파일 오류
- 글로브 패턴 불일치
- 설정 리로드 안 됨

**해결**:

```bash
# 설정 검증
jq . ~/.rrouter/config.json

# 설정 리셋
rrouter config reset

# 데몬 재시작
rrouter restart
```

### 11.6 fsnotify 감시 오류

**증상**:

```
[WATCHER] Failed to watch directory: ...
[WATCHER] Error: ...
```

**원인**:
- 파일 시스템 제한 (inotify fd 부족)
- 권한 문제
- macOS에서 감시자 생성 실패

**해결**:

```bash
# Linux: inotify 제한 증가
echo "fs.inotify.max_user_watches=524288" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p

# 또는 프록시 수동 실행 (fallback 모드)
# 감시자 실패 시 폴백 → 요청마다 파일 읽기
```

---

## 12. 개발 가이드

### 12.1 로컬 개발 설정

```bash
cd /home/forge/project_src/ForgeLab/rrouter

# 빌드
go build -o rrouter ./cmd/rrouter

# Foreground 실행 (로그 확인용)
./rrouter serve

# 다른 터미널에서 테스트
curl http://localhost:8316/health
```

### 12.2 코드 구조

```
cmd/rrouter/
├── cli.go               # Main entry point and CLI routing
├── serve.go             # HTTP request handling, health endpoint
├── daemon.go            # Daemon management (start/stop/status)
├── mode.go              # Mode switching commands
├── config_cmd.go        # Config subcommands
├── proxy_config.go      # Config loading, model matching
├── auto.go              # Auto mode state management
├── watcher.go           # fsnotify watcher
└── help.go              # Help text

config.json              # Default configuration at project root
```

### 12.3 주요 개발 작업

#### 새 모드 추가

1. **config.json에 모드 추가** (프로젝트 루트):

```json
"my-mode": {
  "mappings": [...]
}
```

2. **CLI에 명령 추가** (`cmd/rrouter/mode.go` 또는 `cmd/rrouter/cli.go`):

```go
// Add case in CLI routing
case "my-mode", "mm":
    cmdMyMode()

func cmdMyMode() {
    setMode("my-mode")
    fmt.Println("Mode set to: my-mode")
}
```

3. **빌드 및 테스트**:

```bash
go build -o rrouter ./cmd/rrouter
./rrouter serve  # Foreground
# 다른 터미널에서 테스트
rrouter my-mode
curl http://localhost:8316/health
```

#### Auto mode 로직 수정

파일: `cmd/rrouter/auto.go`

```go
// recordUpstreamResponse() 수정
// - 임계값 조정 (failureThreshold, timeoutThreshold)
// - 쿨다운 기간 변경 (initialCooldown, maxCooldown)
// - 리셋 조건 변경 (success 정의)

// triggerSwitch() 수정
// - 전환 로직 변경
// - 로깅 추가
```

#### 설정 형식 변경

파일: `cmd/rrouter/proxy_config.go`

```go
// Config, ModeConfig, ModelMapping 구조체 변경
// loadConfig(), rewriteModelWithConfig() 업데이트
// 글로브 패턴 로직 변경 (filepath.Match → 다른 방식)
```

### 12.4 테스트

**헬스 엔드포인트 테스트**:

```bash
curl http://localhost:8316/health | jq '.'
```

**모드 전환 테스트**:

```bash
rrouter auto
curl http://localhost:8316/health | jq '.mode'

rrouter ag
curl http://localhost:8316/health | jq '.mode'
```

```bash
# 모델 변환 테스트
# 로그에서 확인
journalctl --user -u rrouter -f | grep "Rewriting"

# 또는 수동 요청
curl -X POST http://localhost:8316/v1/messages \
  -H "Content-Type: application/json" \
  -d '{"model": "claude-sonnet-4-5-20250929"}'
```

**Auto mode 테스트**:

```bash
rrouter auto

# Antigravity 실패 시뮬레이션 (antigravity-proxy 중지)
pkill -f antigravity-proxy

# Claude Code 요청 → 자동 전환
curl http://localhost:8316/health | jq '.autoSwitched'
# true 반환 확인

# antigravity-proxy 재시작
# 쿨다운 후 자동 재시도
```

### 12.5 디버깅 팁

```bash
# 로그 레벨 증가
# 프록시 Foreground 실행 (모든 로그 표시)
rrouter serve

# 또는 systemd 로그
journalctl --user -u rrouter -f -n 100
```

**요청 추적**:

```bash
# 모든 요청 확인
journalctl --user -u rrouter -f | grep "Req #"

# 특정 모드의 요청만
journalctl --user -u rrouter -f | grep "mode: auto"
```

**메모리 프로파일링**:

```bash
# Go 표준 도구 사용
go tool pprof http://localhost:6060/debug/pprof/heap
```

### 12.6 성능 최적화

**Caching**:
- ConfigWatcher가 mode와 config을 메모리에 캐싱
- 요청마다 I/O 없음
- 변경 감지 시만 파일 읽기

**Streaming**:
- `proxy.FlushInterval = -1`로 SSE 스트리밍 활성화
- 청크 단위 즉시 전달

**동시성**:
- `sync.RWMutex`로 읽기/쓰기 분리
- `atomic.Uint64`로 요청 카운팅

---

## 부록

### A. 모드 비교 테이블

| 기능 | Antigravity | Claude | Auto |
|------|-------------|--------|------|
| **모델 변환** | O (Gemini) | X (Passthrough) | O (변환됨) |
| **Thinking** | O | X | O (현재) |
| **비용** | 높음 | 낮음 | 높음 (우선) |
| **응답 속도** | 느림 | 빠름 | 느림 (우선) |
| **자동 복구** | X | X | O |
| **수동 제어** | 필요 | 필요 | 자동 |
| **권장** | 분석/추론 | 빠른 응답 | 일반 사용 |

### B. 환경변수 목록

| 변수 | 기본값 | 설명 | 파일 |
|------|--------|------|------|
| `RROUTER_PORT` | 8316 | 프록시 리스닝 포트 | - |
| `RROUTER_UPSTREAM` | http://localhost:8317 | Upstream URL | - |

### C. API 엔드포인트

| 엔드포인트 | 메소드 | 설명 | 응답 |
|-----------|--------|------|------|
| `/health` | GET | 프록시 상태 및 auto mode 정보 | JSON |
| `/*` | ANY | 모든 요청을 upstream으로 프록시 | Upstream 응답 |

### D. 모드 파일 형식

**위치**: `~/.rrouter/mode`

**유효한 값**:
- `antigravity` - Antigravity 모드
- `claude` - Claude 모드
- `auto` - Auto mode

**예시**:

```
auto
```

### E. 설정 파일 스키마

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "modes": {
      "type": "object",
      "additionalProperties": {
        "type": "object",
        "properties": {
          "mappings": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "match": { "type": "string" },
                "rewrite": { "type": "string" }
              },
              "required": ["match", "rewrite"]
            }
          }
        }
      }
    },
    "defaultMode": { "type": "string" }
  }
}
```

### F. 문제 해결 체크리스트

프록시 실행 안 됨:
- [ ] Go 1.22+ 설치
- [ ] `go build` 성공
- [ ] 포트 8316/8317 사용 중인지 확인
- [ ] `~/.rrouter/` 디렉토리 생성
- [ ] 서비스 활성화 확인

모드 전환 안 됨:
- [ ] 모드 파일 위치 확인 (`~/.rrouter/mode`)
- [ ] 파일 읽기 권한 확인
- [ ] 프록시 health 엔드포인트 응답 확인
- [ ] fsnotify 감시자 로그 확인

Auto mode 작동 안 됨:
- [ ] `~/.rrouter/mode`에 "auto" 기록
- [ ] antigravity-proxy 실행 중인지 확인
- [ ] 헬스 엔드포인트에서 `autoSwitched` 확인
- [ ] 로그에서 AUTO 메시지 확인

Claude Code 연결 안 됨:
- [ ] `~/.claude/settings.json` 설정 확인
- [ ] 프록시 포트 8316 확인
- [ ] Claude Code 재시작
- [ ] 프록시 재시작

---

*문서 생성일: 2026-01-29*
*rrouter 버전: 4.0.0*
*최종 업데이트: 2026-01-30 - 단일 바이너리 통합 아키텍처 (CLI + Proxy), 데몬 관리 통합, install/uninstall QA*
