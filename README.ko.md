# rrouter

**한국어** | [English](README.md)

Claude Code CLI를 위한 경량 라우팅 프록시. 재시작 없이 OAuth 라우팅 모드를 즉시 전환하고, 자동 장애 감지/복구 기능을 제공합니다.

---

## 주요 기능

- **무재시작 모드 전환**: 프록시나 Claude Code CLI를 재시작하지 않고도 라우팅 모드를 즉시 변경
- **자동 장애 감지**: Antigravity 실패 시 자동으로 Claude로 폴백 (양방향 지원)
- **fsnotify 기반 설정 감시**: 파일 시스템 변경을 즉시 반영하며 I/O 오버헤드 없음
- **글로브 패턴 매칭**: 모델명 매핑에서 와일드카드 지원 (`claude-sonnet-*`)
- **세 가지 라우팅 모드**: antigravity (Gemini Thinking), claude (OAuth passthrough), auto (자동 폴백)

---

## 설치

```bash
./install.sh
```

설치 스크립트는 자동으로 다음을 수행합니다:
- 바이너리 빌드 및 `~/bin/` 설치
- 설정 디렉토리 생성 (`~/.rrouter/`)
- 서비스 등록 (Linux: systemd, macOS: launchd)
- 기본 모드를 `auto`로 설정

---

## Claude Code 설정 (필수)

rrouter를 사용하려면 Claude Code CLI가 프록시를 통해 요청을 보내도록 설정해야 합니다.

### 설정 파일 위치

```
~/.claude/settings.json
```

### 권장 설정

`~/.claude/settings.json` 파일을 다음과 같이 편집하세요:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
```

설정 파일이 없는 경우 생성:

```bash
mkdir -p ~/.claude
cat > ~/.claude/settings.json <<'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF
```

---

## 사용법

### 데몬 관리

```bash
rrouter start      # 데몬 시작
rrouter stop       # 데몬 중지
rrouter restart    # 데몬 재시작
rrouter status     # 상태 확인
rrouter serve      # Foreground 실행 (디버깅용)
```

### 모드 전환

```bash
rrouter auto            # Auto 모드 (권장, 자동 폴백)
rrouter antigravity     # Antigravity 모드
rrouter ag              # antigravity 단축 명령

rrouter claude          # Claude OAuth 모드
rrouter c               # claude 단축 명령
```

### 상태 확인

```bash
rrouter status          # 현재 모드 및 데몬 상태
rrouter --check         # 헬스 체크
curl http://localhost:8316/health | jq '.'
```

### 설정 관리

```bash
rrouter config          # 현재 설정 보기
rrouter config edit     # 에디터로 설정 수정
rrouter config reset    # 기본값으로 리셋
rrouter config path     # 설정 파일 경로 표시
```

---

## 세 가지 라우팅 모드

| 모드 | 설명 | 사용 시나리오 |
|------|------|--------------|
| **auto** (권장) | 양방향 자동 폴백 (Antigravity ↔ Claude) | 일반 사용, 자동 복구 필요 시 |
| **antigravity** | Gemini Thinking 모델 사용 | 추론/분석 작업, 높은 품질 필요 |
| **claude** | Claude OAuth 직접 사용 | 빠른 응답, 저비용 |

### Auto Mode 동작

Auto Mode는 지능형 라우팅 모드로, 한쪽 대상이 연속 실패하면 자동으로 다른 쪽으로 전환됩니다.

**장애 감지 규칙**:
- HTTP 429/4xx/5xx: 3회 연속 → 전환
- Timeout: 2회 연속 → 전환
- HTTP 2xx 성공: 카운터 리셋

**쿨다운 정책**:
- 1차 전환: 30분 후 재시도
- 2차 전환: 60분 후 재시도
- 3차 전환: 120분 후 재시도
- 4차 이상: 240분 후 재시도 (최대)

---

## 설정 파일

### ~/.rrouter/mode

현재 라우팅 모드를 저장하는 단순 텍스트 파일입니다.

**유효한 값**: `antigravity`, `claude`, `auto`

**예시**:
```
auto
```

### ~/.rrouter/config.json

모델명 매핑 규칙을 정의하는 JSON 파일입니다.

**예시**:

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

**글로브 패턴 지원**:
- `claude-sonnet-*`: 모든 Sonnet 버전 매칭
- `claude-opus-4-5-*`: Opus 4.5.x 버전 매칭
- `*`: 모든 모델 매칭

---

## CLI 명령어

| 명령어 | 별칭 | 설명 |
|--------|------|------|
| `rrouter serve` | - | Foreground로 실행 |
| `rrouter start` | - | 데몬 시작 |
| `rrouter stop` | - | 데몬 중지 |
| `rrouter restart` | - | 데몬 재시작 |
| `rrouter status` | - | 상태 확인 |
| `rrouter antigravity` | `ag` | Antigravity 모드 전환 |
| `rrouter claude` | `c` | Claude 모드 전환 |
| `rrouter auto` | `a` | Auto 모드 전환 |
| `rrouter --check` | `check`, `health` | 헬스 체크 |
| `rrouter config` | - | 설정 보기 |
| `rrouter config edit` | - | 설정 수정 |
| `rrouter config reset` | - | 설정 리셋 |
| `rrouter help` | `-h`, `--help` | 도움말 |

---

## 환경변수

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `RROUTER_PORT` | 8316 | 프록시 리스닝 포트 |
| `RROUTER_UPSTREAM` | http://localhost:8317 | Upstream URL (clipproxyapi) |

**설정 예시**:

```bash
# 직접 실행
RROUTER_PORT=9000 rrouter serve

# systemd 서비스 파일 (~/.config/systemd/user/rrouter.service)
[Service]
Environment=RROUTER_PORT=9000
Environment=RROUTER_UPSTREAM=http://localhost:9317
```

---

## 트러블슈팅

### 프록시가 실행되지 않음

```bash
rrouter status                      # 상태 확인
tail -f ~/.rrouter/logs/*.log       # 로그 확인
lsof -i :8316                       # 포트 충돌 확인
rrouter restart                     # 재시작
```

### 모드 전환이 안 됨

```bash
cat ~/.rrouter/mode                 # 모드 파일 확인
curl http://localhost:8316/health | jq '.mode'  # 현재 모드 확인
echo "auto" > ~/.rrouter/mode       # 모드 파일 재생성
rrouter restart
```

### Claude Code 연결 안 됨

```bash
cat ~/.claude/settings.json         # 설정 확인
curl http://localhost:8316/health   # 프록시 상태 확인

# 설정 재설정
cat > ~/.claude/settings.json << 'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF
```

### Auto Mode 작동 확인

```bash
curl -s http://localhost:8316/health | jq '.autoSwitch'
tail -f ~/.rrouter/logs/*.log | grep AUTO
```

---

## 아키텍처

```
┌─────────────────┐
│ Claude Code CLI │
│  (port: 8316)   │
└────────┬────────┘
         │ HTTP Request
         │ model: claude-sonnet-4-5-*
         ▼
┌─────────────────┐
│  rrouter:8316   │
│                 │
│  1. 모드 확인    │
│  2. 모델명 변환  │
│  3. Auto 상태   │
│     관리        │
└────────┬────────┘
         │ Modified Request
         │ model: gemini-claude-sonnet-4-5-thinking
         ▼
┌─────────────────┐
│clipproxyapi:8317│
│ OAuth 라우팅    │
└────────┬────────┘
         │
         ▼
  Antigravity / Claude
```

**2단계 변환 시스템**:
1. **rrouter (Stage 1)**: Claude 모델명 → Gemini prefix 모델명
2. **clipproxyapi (Stage 2)**: oauth-model-alias 매칭 → OAuth 채널 선택

---

## 요구사항

- Go 1.22+ (빌드 시에만 필요)
- clipproxyapi (localhost:8317에서 실행 중)
- Linux (systemd) 또는 macOS (launchd)

---

## 참조

- [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) - rrouter가 전달하는 upstream 프록시
- [oh-my-claudecode](https://github.com/Yeachan-Heo/oh-my-claudecode) - Claude Code를 위한 멀티에이전트 오케스트레이션

## 라이선스

MIT

---

**문서 버전**: 2.0.0
**최종 업데이트**: 2026-01-30
**rrouter 버전**: 4.0.0

상세한 내용은 [영문 README](README.md)를 참조하세요.
