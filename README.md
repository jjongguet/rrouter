# rrouter

**한국어 | English**

Claude Code CLI를 위한 경량 라우팅 프록시. 재시작 없이 OAuth 라우팅 모드를 즉시 전환하고, 자동 장애 감지/복구 기능을 제공합니다.

A lightweight routing proxy for Claude Code CLI. Switch OAuth routing modes instantly without restart, with automatic failure detection and recovery.

---

## 목차 / Table of Contents

1. [프로젝트 개요 / Project Overview](#1-프로젝트-개요--project-overview)
2. [아키텍처 다이어그램 / Architecture Diagram](#2-아키텍처-다이어그램--architecture-diagram)
3. [Claude Code 설정 (중요) / Claude Code Configuration (CRITICAL)](#3-claude-code-설정-중요--claude-code-configuration-critical)
4. [clipproxyapi 연동 (중요) / clipproxyapi Integration (CRITICAL)](#4-clipproxyapi-연동-중요--clipproxyapi-integration-critical)
5. [빠른 시작 / Quick Start](#5-빠른-시작--quick-start)
6. [세 가지 라우팅 모드 / Three Routing Modes](#6-세-가지-라우팅-모드--three-routing-modes)
7. [Auto Mode 상세 / Auto Mode Details](#7-auto-mode-상세--auto-mode-details)
8. [설정 파일 / Configuration Files](#8-설정-파일--configuration-files)
9. [CLI 명령어 / CLI Commands](#9-cli-명령어--cli-commands)
10. [Health Endpoint](#10-health-endpoint)
11. [트러블슈팅 / Troubleshooting](#11-트러블슈팅--troubleshooting)
12. [개발 / Development](#12-개발--development)

---

## 1. 프로젝트 개요 / Project Overview

### 한국어

**rrouter**는 Claude Code CLI와 백엔드 API 사이에서 동작하는 **경량 라우팅 프록시**입니다.

#### 핵심 기능

- **무재시작 모드 전환**: 프록시나 Claude Code CLI를 재시작하지 않고도 라우팅 모드를 즉시 변경
- **자동 장애 감지**: Antigravity 실패 시 자동으로 Claude로 폴백
- **fsnotify 기반 설정 감시**: 파일 시스템 변경을 즉시 반영하며 I/O 오버헤드 없음
- **글로브 패턴 매칭**: 모델명 매핑에서 와일드카드 지원 (`claude-sonnet-*`)

#### 기술 스택

- **언어**: Go 1.22+
- **설정 감시**: fsnotify (macOS inode 교체 대응)
- **서비스 관리**: Linux (systemd), macOS (launchd)

### English

**rrouter** is a **lightweight routing proxy** that sits between Claude Code CLI and backend APIs.

#### Key Features

- **Zero-downtime mode switching**: Switch routing modes instantly without restarting proxy or Claude Code CLI
- **Automatic failure detection**: Automatically fallback to Claude when Antigravity fails
- **fsnotify-based configuration watching**: Instant file system change reflection with zero I/O overhead
- **Glob pattern matching**: Wildcard support in model name mapping (`claude-sonnet-*`)

#### Tech Stack

- **Language**: Go 1.22+
- **Config Watching**: fsnotify (macOS inode replacement handling)
- **Service Management**: Linux (systemd), macOS (launchd)

---

## 2. 아키텍처 다이어그램 / Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                     Claude Code CLI                          │
│          ~/.claude/settings.json 에서 설정 / Configured      │
│          env.ANTHROPIC_BASE_URL = http://localhost:8316     │
└────────────────────────────┬────────────────────────────────┘
                             │
                             │ HTTP Request
                             │ POST /v1/messages
                             │ {"model": "claude-sonnet-4-5-20250929", ...}
                             │
                             ▼
┌─────────────────────────────────────────────────────────────┐
│                   rrouter:8316                               │
│                                                              │
│  ┌────────────────────────────────────────────────────────┐ │
│  │  fsnotify Config Watcher (메모리 캐시 / memory cache)  │ │
│  │  ~/.rrouter/mode         (antigravity/claude/auto)     │ │
│  │  ~/.rrouter/config.json  (glob: claude-sonnet-*)       │ │
│  └────────────────────────────────────────────────────────┘ │
│                            │                                 │
│  1. 요청 수신 / Receive request                              │
│  2. 모드 확인 / Check mode (from cache)                      │
│  3. AUTO MODE: 장애 감지 / Failure detection                │
│     - 429/5xx × 3 → switch to Claude                        │
│     - 4xx × 3 → switch to Claude                            │
│     - timeout × 2 → switch to Claude                        │
│     - 2xx → reset counter                                   │
│  4. 모델명 변환 / Rewrite model name (glob match)            │
│     claude-sonnet-* → gemini-claude-sonnet-4-5-thinking     │
│  5. clipproxyapi로 전달 / Forward to upstream                │
│                                                              │
└─────────────────────────┬────────────────────────────────────┘
                          │
                          │ Modified Request
                          │ {"model": "gemini-claude-sonnet-4-5-thinking", ...}
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                      clipproxyapi:8317                       │
│                  (수정 없이 사용 / unchanged)                 │
│                                                              │
│  - oauth-model-alias 매칭 / matching                         │
│  - oauth-excluded-models 필터링 / filtering                  │
│  - Antigravity/Claude OAuth 라우팅 / routing                │
└─────────────────────────┬────────────────────────────────────┘
                          │
                          ▼
                 Antigravity / Claude OAuth
                 (same upstream, model rewrite only)
```

### 동작 흐름 / Flow

1. **Claude Code CLI** → `ANTHROPIC_BASE_URL=http://localhost:8316`으로 요청 전송
2. **rrouter** → 모드 확인, 모델명 변환, Auto mode 상태 관리
3. **clipproxyapi** → OAuth 채널 선택 및 실제 API 호출
4. **Antigravity/Claude** → 최종 응답 생성

---

## 3. Claude Code 설정 (중요) / Claude Code Configuration (CRITICAL)

### 한국어

rrouter를 사용하려면 **Claude Code CLI가 rrouter 프록시를 통해 요청을 보내도록 설정**해야 합니다.

#### 설정 파일 위치

```
~/.claude/settings.json
```

#### 권장 설정 (환경변수 방식)

`~/.claude/settings.json` 파일을 다음과 같이 편집하세요:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
```

#### 이 설정이 하는 일

- **`ANTHROPIC_BASE_URL`**: Claude Code CLI가 API 요청을 보낼 base URL을 지정합니다
- **`http://localhost:8316`**: rrouter가 기본으로 리스닝하는 주소입니다
- 이 설정으로 **모든 Claude API 요청이 rrouter를 거치게 됩니다**

#### 왜 필요한가?

1. **라우팅 제어**: rrouter가 요청을 가로채서 모드에 따라 다른 백엔드로 라우팅할 수 있습니다
2. **모델명 변환**: Antigravity 모드에서 Claude 모델명을 Gemini 모델명으로 자동 변환합니다
3. **자동 장애 복구**: Auto 모드에서 Antigravity 실패 시 자동으로 Claude로 전환합니다

#### 설정 파일이 없는 경우

`~/.claude` 디렉토리가 없으면 생성하세요:

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

#### 기존 설정이 있는 경우

기존 `settings.json`이 있다면 `env` 섹션에 `ANTHROPIC_BASE_URL`만 추가하세요:

```json
{
  "existingSetting": "value",
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316",
    "OTHER_ENV_VAR": "other_value"
  }
}
```

#### 레거시 설정 (비권장)

이전 버전의 Claude Code에서는 다음과 같이 설정했을 수 있습니다:

```json
{
  "anthropicProxyPort": 8316
}
```

**권장사항**: 위의 환경변수 방식(`env.ANTHROPIC_BASE_URL`)을 사용하세요.

#### 설정 적용 확인

```bash
# rrouter 프록시가 실행 중인지 확인
rrouter status

# Health 엔드포인트 확인
curl http://localhost:8316/health

# Claude Code 재시작 (설정 반영)
# 새 터미널에서 claude 명령 실행
```

### English

To use rrouter, you must **configure Claude Code CLI to send requests through the rrouter proxy**.

#### Configuration File Location

```
~/.claude/settings.json
```

#### Recommended Configuration (Environment Variable Method)

Edit `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
```

#### What This Does

- **`ANTHROPIC_BASE_URL`**: Specifies the base URL for Claude Code CLI to send API requests
- **`http://localhost:8316`**: The address where rrouter listens by default
- With this setting, **all Claude API requests will go through rrouter**

#### Why Is This Needed?

1. **Routing Control**: rrouter can intercept requests and route to different backends based on mode
2. **Model Name Transformation**: In Antigravity mode, automatically rewrites Claude model names to Gemini model names
3. **Automatic Failure Recovery**: In Auto mode, automatically switches to Claude when Antigravity fails

#### If Configuration File Doesn't Exist

Create the `~/.claude` directory:

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

#### If Existing Configuration Exists

If you have an existing `settings.json`, just add `ANTHROPIC_BASE_URL` to the `env` section:

```json
{
  "existingSetting": "value",
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316",
    "OTHER_ENV_VAR": "other_value"
  }
}
```

#### Legacy Configuration (Not Recommended)

Older versions of Claude Code may have used:

```json
{
  "anthropicProxyPort": 8316
}
```

**Recommendation**: Use the environment variable method (`env.ANTHROPIC_BASE_URL`) above.

#### Verify Configuration

```bash
# Check if rrouter proxy is running
rrouter status

# Check health endpoint
curl http://localhost:8316/health

# Restart Claude Code (apply settings)
# Run claude command in a new terminal
```

---

## 4. clipproxyapi 연동 (중요) / clipproxyapi Integration (CRITICAL)

### 한국어

rrouter와 clipproxyapi는 **2단계 모델 변환 체계**로 동작합니다.

#### 2단계 변환 프로세스

```
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│ Claude Code  │         │   rrouter    │         │ clipproxyapi │
│     CLI      │         │   (Stage 1)  │         │   (Stage 2)  │
└──────┬───────┘         └──────┬───────┘         └──────┬───────┘
       │                        │                        │
       │ model:                 │                        │
       │ claude-opus-4-5-*      │                        │
       │───────────────────────>│                        │
       │                        │                        │
       │                        │ STAGE 1: rrouter       │
       │                        │ glob match + rewrite   │
       │                        │ claude-opus-* →        │
       │                        │ gemini-claude-opus-    │
       │                        │ 4-5-thinking           │
       │                        │                        │
       │                        │ model:                 │
       │                        │ gemini-claude-opus-    │
       │                        │ 4-5-thinking           │
       │                        │───────────────────────>│
       │                        │                        │
       │                        │                        │ STAGE 2: clipproxyapi
       │                        │                        │ oauth-model-alias
       │                        │                        │ gemini-claude-opus-4-5-thinking →
       │                        │                        │ claude-opus-4-5-thinking
       │                        │                        │ (antigravity 채널)
       │                        │                        │
       │                        │                        │ Antigravity OAuth 호출
       │                        │                        │
```

#### Stage 1: rrouter 모델 변환

**위치**: `~/.rrouter/config.json`

**기능**: Claude 모델명을 Gemini prefix가 붙은 모델명으로 변환

**예시 설정**:

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

**변환 예시**:

| 원본 모델 (Claude Code) | rrouter 변환 후 | 설명 |
|-------------------------|----------------|------|
| `claude-sonnet-4-5-20250929` | `gemini-claude-sonnet-4-5-thinking` | glob `claude-sonnet-*` 매칭 |
| `claude-opus-4-5-20251101` | `gemini-claude-opus-4-5-thinking` | glob `claude-opus-*` 매칭 |
| `claude-haiku-4-5-20251001` | `gemini-3-flash-preview` | glob `claude-haiku-*` 매칭 |

**왜 `gemini-` prefix를 붙이는가?**

rrouter는 **clipproxyapi의 `oauth-model-alias`와 매칭시키기 위해** `gemini-` prefix를 사용합니다. 이를 통해 clipproxyapi가 어떤 OAuth 채널로 라우팅할지 결정할 수 있습니다.

#### Stage 2: clipproxyapi oauth-model-alias

**위치**: clipproxyapi의 `config.yaml`

**기능**: `gemini-` prefix가 붙은 모델을 실제 OAuth 제공자로 매핑

**clipproxyapi 설정 예시**:

```yaml
oauth-model-alias:
  gemini-cli:
    - match: "gemini-claude-sonnet-4-5-thinking"
      target: "claude-sonnet-4-5-thinking"
      channel: "antigravity"
    - match: "gemini-claude-opus-4-5-thinking"
      target: "claude-opus-4-5-thinking"
      channel: "antigravity"
    - match: "gemini-3-flash-preview"
      target: "gemini-2.0-flash-thinking-exp-1219"
      channel: "antigravity"
```

#### oauth-model-alias vs oauth-excluded-models

**clipproxyapi**는 두 가지 모델 제어 메커니즘을 제공합니다:

| 설정 | 기능 | 사용 목적 |
|------|------|-----------|
| **oauth-model-alias** | 모델 추가 및 이름 변경 | 새 모델 추가, 커스텀 라우팅 |
| **oauth-excluded-models** | 모델 제거 및 숨김 | 특정 모델 비활성화 |

#### oauth-excluded-models 상세

**위치**: clipproxyapi의 `config.yaml`

**기능**: 특정 OAuth 채널에서 특정 모델을 제외

**와일드카드 지원**:

```yaml
oauth-excluded-models:
  gemini-cli:
    - "gemini-2.5-pro"              # 정확히 일치 / Exact match
    - "gemini-2.5-*"                # prefix 매칭 / Prefix match
    - "*-preview"                   # suffix 매칭 / Suffix match
    - "*flash*"                     # substring 매칭 / Substring match
  antigravity:
    - "gemini-3-pro-preview"
  claude:
    - "claude-3-5-haiku-20241022"
```

**와일드카드 패턴 표**:

| 패턴 | 의미 | 예시 매칭 |
|------|------|-----------|
| `gemini-2.5-pro` | 정확히 일치 | `gemini-2.5-pro` only |
| `gemini-2.5-*` | prefix 매칭 | `gemini-2.5-flash`, `gemini-2.5-pro` |
| `*-preview` | suffix 매칭 | `gemini-3-pro-preview`, `claude-preview` |
| `*flash*` | substring 매칭 | `gemini-2.5-flash-lite`, `flash-model` |

#### 전체 변환 흐름 예시

```
1. Claude Code CLI 요청:
   {"model": "claude-opus-4-5-20251101"}

2. rrouter (Stage 1):
   - 모드 확인: "antigravity"
   - glob 매칭: "claude-opus-*" 매치
   - 변환: "gemini-claude-opus-4-5-thinking"

3. clipproxyapi (Stage 2):
   - oauth-model-alias 매칭: "gemini-claude-opus-4-5-thinking"
   - target: "claude-opus-4-5-thinking"
   - channel: "antigravity"
   - oauth-excluded-models 체크: 제외되지 않음

4. Antigravity OAuth 호출:
   - 모델: "claude-opus-4-5-thinking"
   - 제공자: Antigravity
```

#### 통합 설정 예시

**rrouter** (`~/.rrouter/config.json`):

```json
{
  "modes": {
    "antigravity": {
      "mappings": [
        {"match": "claude-sonnet-*", "rewrite": "gemini-claude-sonnet-4-5-thinking"},
        {"match": "claude-opus-*", "rewrite": "gemini-claude-opus-4-5-thinking"}
      ]
    }
  }
}
```

**clipproxyapi** (`config.yaml`):

```yaml
oauth-model-alias:
  gemini-cli:
    - match: "gemini-claude-sonnet-4-5-thinking"
      target: "claude-sonnet-4-5-thinking"
      channel: "antigravity"
    - match: "gemini-claude-opus-4-5-thinking"
      target: "claude-opus-4-5-thinking"
      channel: "antigravity"

oauth-excluded-models:
  antigravity:
    - "claude-3-5-haiku-*"  # Haiku 모델 제외
```

### English

rrouter and clipproxyapi work together in a **two-stage model transformation system**.

#### Two-Stage Transformation Process

```
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│ Claude Code  │         │   rrouter    │         │ clipproxyapi │
│     CLI      │         │   (Stage 1)  │         │   (Stage 2)  │
└──────┬───────┘         └──────┬───────┘         └──────┬───────┘
       │                        │                        │
       │ model:                 │                        │
       │ claude-opus-4-5-*      │                        │
       │───────────────────────>│                        │
       │                        │                        │
       │                        │ STAGE 1: rrouter       │
       │                        │ glob match + rewrite   │
       │                        │ claude-opus-* →        │
       │                        │ gemini-claude-opus-    │
       │                        │ 4-5-thinking           │
       │                        │                        │
       │                        │ model:                 │
       │                        │ gemini-claude-opus-    │
       │                        │ 4-5-thinking           │
       │                        │───────────────────────>│
       │                        │                        │
       │                        │                        │ STAGE 2: clipproxyapi
       │                        │                        │ oauth-model-alias
       │                        │                        │ gemini-claude-opus-4-5-thinking →
       │                        │                        │ claude-opus-4-5-thinking
       │                        │                        │ (antigravity channel)
       │                        │                        │
       │                        │                        │ Antigravity OAuth call
       │                        │                        │
```

#### Stage 1: rrouter Model Transformation

**Location**: `~/.rrouter/config.json`

**Function**: Transform Claude model names to Gemini-prefixed model names

**Example Configuration**:

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

**Transformation Examples**:

| Original Model (Claude Code) | After rrouter Transform | Description |
|-----------------------------|------------------------|-------------|
| `claude-sonnet-4-5-20250929` | `gemini-claude-sonnet-4-5-thinking` | glob `claude-sonnet-*` matched |
| `claude-opus-4-5-20251101` | `gemini-claude-opus-4-5-thinking` | glob `claude-opus-*` matched |
| `claude-haiku-4-5-20251001` | `gemini-3-flash-preview` | glob `claude-haiku-*` matched |

**Why the `gemini-` prefix?**

rrouter uses the `gemini-` prefix to **match with clipproxyapi's `oauth-model-alias`**. This allows clipproxyapi to determine which OAuth channel to route to.

#### Stage 2: clipproxyapi oauth-model-alias

**Location**: clipproxyapi's `config.yaml`

**Function**: Map `gemini-`-prefixed models to actual OAuth providers

**clipproxyapi Configuration Example**:

```yaml
oauth-model-alias:
  gemini-cli:
    - match: "gemini-claude-sonnet-4-5-thinking"
      target: "claude-sonnet-4-5-thinking"
      channel: "antigravity"
    - match: "gemini-claude-opus-4-5-thinking"
      target: "claude-opus-4-5-thinking"
      channel: "antigravity"
    - match: "gemini-3-flash-preview"
      target: "gemini-2.0-flash-thinking-exp-1219"
      channel: "antigravity"
```

#### oauth-model-alias vs oauth-excluded-models

**clipproxyapi** provides two model control mechanisms:

| Setting | Function | Purpose |
|---------|----------|---------|
| **oauth-model-alias** | Add and rename models | Add new models, custom routing |
| **oauth-excluded-models** | Remove and hide models | Disable specific models |

#### oauth-excluded-models Details

**Location**: clipproxyapi's `config.yaml`

**Function**: Exclude specific models from specific OAuth channels

**Wildcard Support**:

```yaml
oauth-excluded-models:
  gemini-cli:
    - "gemini-2.5-pro"              # Exact match
    - "gemini-2.5-*"                # Prefix match
    - "*-preview"                   # Suffix match
    - "*flash*"                     # Substring match
  antigravity:
    - "gemini-3-pro-preview"
  claude:
    - "claude-3-5-haiku-20241022"
```

**Wildcard Pattern Table**:

| Pattern | Meaning | Example Matches |
|---------|---------|-----------------|
| `gemini-2.5-pro` | Exact match | `gemini-2.5-pro` only |
| `gemini-2.5-*` | Prefix match | `gemini-2.5-flash`, `gemini-2.5-pro` |
| `*-preview` | Suffix match | `gemini-3-pro-preview`, `claude-preview` |
| `*flash*` | Substring match | `gemini-2.5-flash-lite`, `flash-model` |

#### Complete Transformation Flow Example

```
1. Claude Code CLI request:
   {"model": "claude-opus-4-5-20251101"}

2. rrouter (Stage 1):
   - Check mode: "antigravity"
   - Glob match: "claude-opus-*" matches
   - Transform: "gemini-claude-opus-4-5-thinking"

3. clipproxyapi (Stage 2):
   - oauth-model-alias match: "gemini-claude-opus-4-5-thinking"
   - target: "claude-opus-4-5-thinking"
   - channel: "antigravity"
   - oauth-excluded-models check: not excluded

4. Antigravity OAuth call:
   - model: "claude-opus-4-5-thinking"
   - provider: Antigravity
```

#### Integrated Configuration Example

**rrouter** (`~/.rrouter/config.json`):

```json
{
  "modes": {
    "antigravity": {
      "mappings": [
        {"match": "claude-sonnet-*", "rewrite": "gemini-claude-sonnet-4-5-thinking"},
        {"match": "claude-opus-*", "rewrite": "gemini-claude-opus-4-5-thinking"}
      ]
    }
  }
}
```

**clipproxyapi** (`config.yaml`):

```yaml
oauth-model-alias:
  gemini-cli:
    - match: "gemini-claude-sonnet-4-5-thinking"
      target: "claude-sonnet-4-5-thinking"
      channel: "antigravity"
    - match: "gemini-claude-opus-4-5-thinking"
      target: "claude-opus-4-5-thinking"
      channel: "antigravity"

oauth-excluded-models:
  antigravity:
    - "claude-3-5-haiku-*"  # Exclude Haiku models
```

---

## 5. 빠른 시작 / Quick Start

### 한국어

```bash
# 1. 설치
./install.sh

# 2. 기본 모드는 이미 auto로 설정됨 (자동 폴백)
# 필요시 수동 전환:
rrouter auto             # Auto-switch (양방향 자동 전환)

# 3. Claude Code 사용 시작
# 재시작 불필요! 바로 사용 가능합니다.
```

### English

```bash
# 1. Installation
./install.sh

# 2. Default mode is already set to auto (bidirectional fallback)
# Switch manually if needed:
rrouter auto             # Auto-switch (bidirectional automatic switching)

# 3. Start using Claude Code
# No restart needed! Ready to use immediately.
```

---

## 6. 세 가지 라우팅 모드 / Three Routing Modes

### 한국어

| 모드 | 설명 | 사용 시나리오 | 변환 |
|------|------|--------------|------|
| **antigravity** | Gemini Thinking 모델 사용 | 추론/분석 작업, 높은 품질 필요 | O (Claude → Gemini) |
| **claude** | Claude OAuth 직접 사용 | 빠른 응답, 저비용 | X (passthrough) |
| **auto** (권장) | 양방향 자동 폴백 (Antigravity ↔ Claude) | 일반 사용, 자동 복구 | O (현재 타겟) |

#### 모드 전환

```bash
# Antigravity (thinking models)
rrouter antigravity
rrouter ag              # shorthand

# Claude (OAuth passthrough)
rrouter claude
rrouter c               # shorthand

# Auto (Antigravity-first with Claude fallback)
rrouter auto
rrouter a               # shorthand
```

### English

| Mode | Description | Use Case | Transformation |
|------|-------------|----------|----------------|
| **antigravity** | Use Gemini Thinking models | Reasoning/analysis, high quality needed | Yes (Claude → Gemini) |
| **claude** | Direct Claude OAuth usage | Fast response, low cost | No (passthrough) |
| **auto** (recommended) | Bidirectional automatic fallback (Antigravity ↔ Claude) | General use, automatic recovery | Yes (current target) |

#### Mode Switching

```bash
# Antigravity (thinking models)
rrouter antigravity
rrouter ag              # shorthand

# Claude (OAuth passthrough)
rrouter claude
rrouter c               # shorthand

# Auto (Antigravity-first with Claude fallback)
rrouter auto
rrouter a               # shorthand
```

---

## 7. Auto Mode 상세 / Auto Mode Details

### 한국어

**Auto Mode**는 양방향 자동 폴백을 지원하는 지능형 라우팅 모드입니다. 한쪽 대상이 연속 실패하면 다른 쪽으로 자동 전환되며, 쿨다운 후 원래 대상 재시도를 시도합니다.

#### 장애 탐지 규칙 (양방향)

| 오류 유형 | 임계값 | 동작 |
|----------|--------|------|
| HTTP 429 (rate limit) | 3회 연속 | 다른 대상으로 전환 |
| HTTP 4xx (client error) | 3회 연속 | 다른 대상으로 전환 |
| HTTP 5xx (server error) | 3회 연속 | 다른 대상으로 전환 |
| Timeout | 2회 연속 | 다른 대상으로 전환 |
| HTTP 2xx (성공) | 1회 | 카운터 리셋 |

**참고**: 양방향 폴백이므로 Antigravity가 실패하면 Claude로, Claude가 실패하면 Antigravity로 전환됩니다.

#### 쿨다운 및 재시도

| 시도 | 쿨다운 기간 | 트리거 |
|------|-----------|--------|
| 1차 전환 | 30분 | 초기 실패 |
| 2차 전환 | 60분 | 30분 후 재시도 시 재실패 |
| 3차 전환 | 120분 | 다시 2배 증가 |
| 4차 이상 | 240분 (최대) | 4시간 상한선 |

**양방향 작동**: 같은 실패 감지 규칙이 양 대상에 모두 적용되므로, Antigravity가 실패하면 Claude로 전환되고, 그 후 Claude도 실패하면 Antigravity로 다시 전환됩니다. 각 전환 후 동일한 쿨다운 정책이 적용됩니다.

#### 쿨다운 리셋 조건

- **수동 전환**: `rrouter ag` 또는 `rrouter claude` → 모든 상태 초기화
- **프록시 재시작**: Antigravity부터 새로 시작
- **지속적 정상**: 현재 쿨다운의 2배 시간 동안 정상 작동 → 30분으로 리셋

#### 상태 전환 다이어그램 (양방향)

```
                    ┌─────────────────┐
                    │   Antigravity   │
                    │   (default)     │
                    └────────┬────────┘
                             │
                  4xx/5xx ×3 │ timeout ×2
                             │
                             ▼
                    ┌─────────────────┐
                    │     Claude      │
                    │   (fallback)    │
                    └────────┬────────┘
                             │
                    4xx/5xx ×3 │ timeout ×2
                             │
                             ▼
              ┌──────────────────────────────┐
              │    Back to Antigravity       │
              │   (bidirectional switch)     │
              └──────┬───────────────────────┘
                     │
                     │ Cooldown expires
                     │ (30m ~ 240m)
                     │
                     ▼
         ┌───────────────────────────┐
         │   Retry Original Target   │
         │   (with backoff policy)   │
         └───────────────────────────┘
              │                  │
    Success   │                  │ Failure
    (2xx)     │                  │ (4xx/5xx/timeout)
              │                  │
              ▼                  ▼
         ┌─────────────┐  ┌──────────────────┐
         │ Hold state  │  │ Switch to other  │
         │ (cooldown   │  │ (double cooldown)│
         │ reset)      │  └──────────────────┘
         └─────────────┘
```

**특징**:
- 양방향 폴백: Antigravity ↔ Claude 양쪽 모두 가능
- 동일한 규칙: 4xx/5xx ×3, timeout ×2 (양 방향 동일)
- 쿨다운 정책: 첫 전환 30분, 이후 2배씩 증가 (최대 240분)

#### 제약사항

- **탐지 범위**: Pre-stream errors only (SSE 스트림 중간 오류 탐지 불가)
- **상태 저장**: In-memory only (프록시 재시작 시 초기화)
- **라우팅 방식**: 같은 upstream, 모델명 재작성만 (URL 전환 없음)

### English

**Auto Mode** is an intelligent routing mode that supports bidirectional automatic fallback. When one target fails consecutively, the system automatically switches to the other, and after cooldown, it retries the original target.

#### Failure Detection Rules (Bidirectional)

| Error Type | Threshold | Action |
|-----------|-----------|--------|
| HTTP 429 (rate limit) | 3 consecutive | Switch to other target |
| HTTP 4xx (client error) | 3 consecutive | Switch to other target |
| HTTP 5xx (server error) | 3 consecutive | Switch to other target |
| Timeout | 2 consecutive | Switch to other target |
| HTTP 2xx (success) | 1 | Reset counter |

**Note**: Bidirectional fallback means Antigravity failure switches to Claude, and Claude failure switches back to Antigravity.

#### Cooldown and Retry

| Attempt | Cooldown Duration | Trigger |
|---------|------------------|---------|
| 1st switch | 30 minutes | Initial failure |
| 2nd switch | 60 minutes | Retry after 30min fails again |
| 3rd switch | 120 minutes | Double again |
| 4th+ switch | 240 minutes (max) | Capped at 4 hours |

**Bidirectional Operation**: The same failure detection rules apply to both targets, so Antigravity failure switches to Claude, and Claude failure switches back to Antigravity. The same cooldown policy applies after each switch.

#### Cooldown Reset Conditions

- **Manual override**: `rrouter ag` or `rrouter claude` → Clear all state
- **Proxy restart**: Fresh start on Antigravity
- **Sustained health**: 2× current cooldown of successful operation → Reset to 30min

#### State Transition Diagram (Bidirectional)

```
                    ┌─────────────────┐
                    │   Antigravity   │
                    │   (default)     │
                    └────────┬────────┘
                             │
                  4xx/5xx ×3 │ timeout ×2
                             │
                             ▼
                    ┌─────────────────┐
                    │     Claude      │
                    │   (fallback)    │
                    └────────┬────────┘
                             │
                    4xx/5xx ×3 │ timeout ×2
                             │
                             ▼
              ┌──────────────────────────────┐
              │    Back to Antigravity       │
              │   (bidirectional switch)     │
              └──────┬───────────────────────┘
                     │
                     │ Cooldown expires
                     │ (30m ~ 240m)
                     │
                     ▼
         ┌───────────────────────────┐
         │   Retry Original Target   │
         │   (with backoff policy)   │
         └───────────────────────────┘
              │                  │
    Success   │                  │ Failure
    (2xx)     │                  │ (4xx/5xx/timeout)
              │                  │
              ▼                  ▼
         ┌─────────────┐  ┌──────────────────┐
         │ Hold state  │  │ Switch to other  │
         │ (cooldown   │  │ (double cooldown)│
         │ reset)      │  └──────────────────┘
         └─────────────┘
```

**Features**:
- Bidirectional fallback: Antigravity ↔ Claude both directions supported
- Same rules: 4xx/5xx ×3, timeout ×2 (identical in both directions)
- Cooldown policy: 1st switch 30min, then doubles each time (max 240min)

#### Limitations

- **Detection Scope**: Pre-stream errors only (cannot detect SSE mid-stream failures)
- **State Persistence**: In-memory only (reset on proxy restart)
- **Routing Method**: Same upstream, model name rewriting only (not URL switching)

---

## 8. 설정 파일 / Configuration Files

### 한국어

#### ~/.rrouter/mode

**형식**: 단순 텍스트 파일

**유효한 값**: `antigravity`, `claude`, `auto`

**예시**:

```
auto
```

#### ~/.rrouter/config.json

**필수성**: 선택사항 (없으면 내장 기본값 사용)

**형식**: JSON

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

**글로브 패턴 지원** (`filepath.Match`):

| 패턴 | 매칭 대상 |
|------|---------|
| `claude-sonnet-*` | 모든 Sonnet 버전 (`claude-sonnet-4-5-20250929`, etc.) |
| `claude-opus-4-5-*` | Opus 4.5.x 버전 |
| `claude-haiku-*` | 모든 Haiku 버전 |
| `*` | 모든 모델 |

#### 환경변수

| 변수 | 기본값 | 설명 |
|------|--------|------|
| `RROUTER_PORT` | 8316 | 프록시 리스닝 포트 |
| `RROUTER_UPSTREAM` | http://localhost:8317 | Upstream URL (clipproxyapi) |

**설정 방법**:

```bash
# 직접 실행
RROUTER_PORT=9000 rrouter serve

# systemd 서비스 파일
# ~/.config/systemd/user/rrouter.service
[Service]
Environment=RROUTER_PORT=9000
Environment=RROUTER_UPSTREAM=http://localhost:9317
```

### English

#### ~/.rrouter/mode

**Format**: Simple text file

**Valid values**: `antigravity`, `claude`, `auto`

**Example**:

```
auto
```

#### ~/.rrouter/config.json

**Required**: Optional (uses embedded defaults if missing)

**Format**: JSON

**Example**:

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

**Glob Pattern Support** (`filepath.Match`):

| Pattern | Matches |
|---------|---------|
| `claude-sonnet-*` | All Sonnet versions (`claude-sonnet-4-5-20250929`, etc.) |
| `claude-opus-4-5-*` | Opus 4.5.x versions |
| `claude-haiku-*` | All Haiku versions |
| `*` | All models |

#### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `RROUTER_PORT` | 8316 | Proxy listening port |
| `RROUTER_UPSTREAM` | http://localhost:8317 | Upstream URL (clipproxyapi) |

**Configuration**:

```bash
# Direct execution
RROUTER_PORT=9000 rrouter serve

# systemd service file
# ~/.config/systemd/user/rrouter.service
[Service]
Environment=RROUTER_PORT=9000
Environment=RROUTER_UPSTREAM=http://localhost:9317
```

---

## 9. CLI 명령어 / CLI Commands

### 한국어

| 명령어 | 별칭 | 설명 |
|--------|------|------|
| `rrouter serve` | - | Proxy server를 foreground로 실행 |
| `rrouter start` | - | Proxy daemon을 background로 시작 |
| `rrouter stop` | - | Proxy daemon 중지 |
| `rrouter restart` | - | Proxy daemon 재시작 |
| `rrouter status` | - | 현재 모드 및 데몬 상태 표시 |
| `rrouter antigravity` | `ag` | Antigravity 모드로 전환 |
| `rrouter claude` | `c` | Claude OAuth 모드로 전환 |
| `rrouter auto` | `a` | Auto mode 활성화 |
| `rrouter --check` | `check`, `health` | 헬스 체크 |
| `rrouter config` | - | 현재 config.json 보기 |
| `rrouter config edit` | - | 에디터로 config.json 수정 |
| `rrouter config reset` | - | config.json 기본값으로 리셋 |
| `rrouter config path` | - | config.json 파일 경로 표시 |
| `rrouter help` | `--help`, `-h` | 도움말 표시 |

### English

| Command | Alias | Description |
|---------|-------|-------------|
| `rrouter serve` | - | Run proxy server in foreground |
| `rrouter start` | - | Start proxy daemon in background |
| `rrouter stop` | - | Stop proxy daemon |
| `rrouter restart` | - | Restart proxy daemon |
| `rrouter status` | - | Show current mode and daemon status |
| `rrouter antigravity` | `ag` | Switch to Antigravity mode |
| `rrouter claude` | `c` | Switch to Claude OAuth mode |
| `rrouter auto` | `a` | Activate Auto mode |
| `rrouter --check` | `check`, `health` | Health check |
| `rrouter config` | - | View current config.json |
| `rrouter config edit` | - | Edit config.json with editor |
| `rrouter config reset` | - | Reset config.json to defaults |
| `rrouter config path` | - | Show config.json file path |
| `rrouter help` | `--help`, `-h` | Show help |

---

## 10. Health Endpoint

### 한국어

**엔드포인트**: `GET http://localhost:8316/health`

**응답 예시**:

```json
{
  "status": "ok",
  "mode": "auto",
  "currentTarget": "antigravity",
  "requestCount": 42,
  "listenAddr": ":8316",
  "upstreamURL": "http://localhost:8317",
  "defaultMode": "claude",
  "autoSwitch": {
    "enabled": true,
    "failureCount": 0,
    "timeoutCount": 0,
    "inCooldown": false,
    "cooldownMinutes": 30,
    "nextRetryTime": "2026-01-30T10:30:00Z"
  }
}
```

**사용 예시**:

```bash
# CLI 도구로 확인
rrouter --check
rrouter check
rrouter health

# 직접 API 호출
curl http://localhost:8316/health | jq '.'

# 현재 Auto 상태만 확인
curl -s http://localhost:8316/health | jq '.autoSwitch'
```

### English

**Endpoint**: `GET http://localhost:8316/health`

**Response Example**:

```json
{
  "status": "ok",
  "mode": "auto",
  "currentTarget": "antigravity",
  "requestCount": 42,
  "listenAddr": ":8316",
  "upstreamURL": "http://localhost:8317",
  "defaultMode": "claude",
  "autoSwitch": {
    "enabled": true,
    "failureCount": 0,
    "timeoutCount": 0,
    "inCooldown": false,
    "cooldownMinutes": 30,
    "nextRetryTime": "2026-01-30T10:30:00Z"
  }
}
```

**Usage Examples**:

```bash
# CLI tool
rrouter --check
rrouter check
rrouter health

# Direct API call
curl http://localhost:8316/health | jq '.'

# Check Auto state only
curl -s http://localhost:8316/health | jq '.autoSwitch'
```

---

## 11. 트러블슈팅 / Troubleshooting

### 한국어

#### 프록시가 실행되지 않음

```bash
# 상태 확인
rrouter status

# 로그 확인 (macOS)
tail -f ~/.rrouter/logs/*.log

# 포트 충돌 확인
lsof -i :8316
lsof -i :8317
```

**해결**:

```bash
# 데몬 재시작
rrouter restart

# 또는 직접 실행 (로그 확인용)
rrouter serve
```

#### 모드 전환이 안 됨

```bash
# 모드 파일 확인
cat ~/.rrouter/mode

# Health 확인
curl http://localhost:8316/health | jq '.mode'
```

**해결**:

```bash
# 모드 파일 재생성
echo "claude" > ~/.rrouter/mode

# 데몬 재시작
rrouter restart
```

#### Claude Code 연결 안 됨

```bash
# 설정 확인
cat ~/.claude/settings.json

# 프록시 health 확인
curl http://localhost:8316/health
```

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
```

#### Auto mode가 작동하지 않음

```bash
# Auto 상태 확인
curl -s http://localhost:8316/health | jq '.autoSwitch'

# 로그 확인
tail -f ~/.rrouter/logs/*.log | grep AUTO
```

### English

#### Proxy Not Running

```bash
# Check status
rrouter status

# Check logs (macOS)
tail -f ~/.rrouter/logs/*.log

# Check port conflicts
lsof -i :8316
lsof -i :8317
```

**Solution**:

```bash
# Restart daemon
rrouter restart

# Or run directly (for log inspection)
rrouter serve
```

#### Mode Switching Not Working

```bash
# Check mode file
cat ~/.rrouter/mode

# Check health
curl http://localhost:8316/health | jq '.mode'
```

**Solution**:

```bash
# Recreate mode file
echo "claude" > ~/.rrouter/mode

# Restart daemon
rrouter restart
```

#### Claude Code Connection Issues

```bash
# Check settings
cat ~/.claude/settings.json

# Check proxy health
curl http://localhost:8316/health
```

**Solution**:

```bash
# Reset settings
cat > ~/.claude/settings.json << 'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:8316"
  }
}
EOF

# Restart Claude Code
```

#### Auto Mode Not Working

```bash
# Check Auto state
curl -s http://localhost:8316/health | jq '.autoSwitch'

# Check logs
tail -f ~/.rrouter/logs/*.log | grep AUTO
```

---

## 12. 개발 / Development

### 한국어

#### 빌드

```bash
cd /path/to/rrouter
go build -o rrouter ./cmd/rrouter
```

#### 테스트

```bash
# Foreground 실행
./rrouter serve

# 다른 터미널에서
curl http://localhost:8316/health
```

#### 파일 구조

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

### English

#### Build

```bash
cd /path/to/rrouter
go build -o rrouter ./cmd/rrouter
```

#### Testing

```bash
# Run in foreground
./rrouter serve

# In another terminal
curl http://localhost:8316/health
```

#### File Structure

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

---

## 요구사항 / Requirements

- Go 1.22+ (build only)
- clipproxyapi running on localhost:8317
- Linux (systemd) or macOS (launchd)

## 라이선스 / License

MIT

---

**Documentation Version**: 2.0.0
**Last Updated**: 2026-01-30
**rrouter Version**: 4.0.0
