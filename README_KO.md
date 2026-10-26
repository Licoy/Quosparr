[English](./README.md) · [简体中文](./README_ZH.md) · [繁體中文](./README_ZH_HANT.md) · 한국어 · [日本語](./README_JA.md) · [Русский](./README_RU.md) · [Français](./README_FR.md)

<div align="center">

<img src="Resources/AppIcon.png" alt="Quosparr 로고" width="128" height="128">

# Quosparr

<sub><i>이름은 <b>Quo</b>ta(할당량) + <b>Sparr</b>ow(참새)에서 왔습니다</i></sub>

메뉴 막대에 앉은 작은 참새가 Codex, Claude, Grok 할당량을 지켜봅니다.

Quosparr는 macOS, Windows, Linux용 네이티브 메뉴 막대 / 시스템 트레이 앱입니다. Codex, Claude Code, Grok 구독 할당량, reset credits, API 환산 비용, 로컬 세션을 보여 주며 공급자별로 계정을 따로 관리합니다. 세 공급자의 전역 Skill, MCP 서버, 로컬 세션도 한곳에서 관리할 수 있습니다.

</div>

## 주요 기능

**Codex**

- 5시간, 주간 및 추가 창의 남은 할당량과 현재 계정, 구독 등급, 만료일.
- 리셋 크레딧 수, 상태, 만료 시각.
- 공식 주간 사용률과 일별 Credits로 주간 한도를 추정하고, 이전 주와 비교해 할당량 삭감 여부를 확인합니다.
- Codex 초기화 업데이트: 오늘 Codex 속도 제한이 리셋되었는지를 자체 새로고침 간격으로 확인합니다([Did Codex Reset](https://didcodexreset.com) 데이터).
- 공급자 계정: 브라우저 로그인, 로컬 `auth.json` 가져오기, 토큰 / JSON 붙여넣기(`/auth/session` 포함), 파일 가져오기, API 키 추가. 확인 후 전환하면 `~/.codex/auth.json`을 원자적으로 씁니다.
- API 계정: Base URL + API 키(Base URL을 비우면 OpenAI 공식 API)이며, 사이트에서 모델 목록을 가져와 모델을 고를 수 있습니다. 전환 시 `auth.json`과 `config.toml`의 `quosparr` 제공자를 기록하고(먼저 백업), new-api 계열 또는 OpenAI 호환 중계 사이트의 잔액을 계정 목록, 패널, 상태 표시줄, 플로팅 창, 위젯에 표시합니다. `config.toml`에서 이미 중계 사이트를 선택한 키 로그인은 해당 사이트와 모델로 자동 인식합니다. 사용 중인 계정을 편집할 때 키를 교체할 수 있고(비우면 유지) 설정 파일을 바로 다시 쓸지 묻습니다. 다시 쓴 뒤에는 Codex를 재시작해야 합니다.
- 최근 세션, 프로젝트, 상태와 로컬 세션 인덱스 복구.

**Claude**

- Claude Code 구독 사용량의 5시간 창, 7일 창, 모델별 주간 한도, 추가 사용량을 봅니다. 이 값은 공개된 결제 API가 아닙니다.
- Claude Code 로컬 세션 로그로 토큰 사용량, API 환산 비용(`anthropic-builtin-2026-10-14`), 최근 세션, 출력 속도를 계산합니다. 같은 답변은 한 번만 셉니다. 알 수 없는 모델은 정확한 비용을 주지 않습니다. 로그에 첫 토큰 시간이 없어 그 칸은 “—”이고, 속도는 처음부터 끝까지의 시간으로 계산합니다.
- 구독 계정은 CLI를 먼저 실행하지 않고 브라우저로 로그인하거나 현재 로그인 가져오기, 자격 증명 붙여넣기, 파일 가져오기를 사용할 수 있습니다. 브라우저 로그인은 취소와 콜백 붙여넣기를 지원하며 계정 라이브러리에만 추가합니다. Claude Code에서 사용하려면 명시적으로 전환해야 합니다. Quosparr는 현재 계정을 새로고침하지 않습니다. 로그인이 만료되면 `claude`를 한 번 실행하세요. 다른 관리 계정은 계정 라이브러리 사본만 새로고칩니다.
- macOS에서 공식 자격 증명은 키체인에 있고 `/usr/bin/security`로 읽고 씁니다. Windows와 Linux는 `<config>/.credentials.json`입니다. 전환은 `claudeAiOauth`만 바꾸고 MCP 토큰은 유지하며, `.claude.json`의 `oauthAccount`만 갱신합니다. 실행 중인 Claude Code는 종료하지 않으며 이전 로그인을 다시 쓸 수 있어 전환 전에 경고합니다.
- API 계정: Base URL + API 키이며, 사이트에서 모델 목록을 가져와 Opus / Sonnet / Haiku / Fable 모델을 각각 매핑할 수 있습니다. 전환 시 `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`과 매핑만 `settings.json`에 기록하고(먼저 백업) 구독 로그인은 그대로 둡니다. 구독 계정으로 돌아가면 이 키들을 제거합니다. 중계 사이트 잔액은 Claude 사용량이 표시되는 모든 곳에 나타납니다. `settings.json`에 이미 설정된 중계 구성은 자동으로 가져옵니다. 사용 중인 계정을 편집할 때 키를 교체할 수 있고(비우면 유지) `settings.json`을 바로 다시 쓸지 묻습니다. 다시 쓴 뒤에는 Claude Code를 재시작해야 합니다.

- **Claude Desktop 구독**: 두 계정 목록에서 연결할 수 있습니다. 처음에는 표시된 계정과 조직에 직접 로그인한 뒤 Quosparr에서 명시적으로 확인하세요. 매번 작업을 저장하고 Desktop 종료와 로그인·설정·로컬 상태를 포함한 전체 구성의 저장/복원에 동의해야 합니다. 연결을 완료하거나 취소하면 이전 구성을 복원합니다. 연동 전환은 기본적으로 꺼져 있으며 현재 CLI 계정도 연결된 Desktop을 별도로 열 수 있습니다. CLI와 Desktop 결과는 따로 표시하며 시작 성공을 신원 확인으로 간주하지 않습니다.
- Desktop 자체의 기본 암호화 로그인 데이터가 포함된 프로필은 `~/.quosparr/claude-desktop`에 비공개 저장합니다(Windows는 현재 사용자 전용 protected ACL). 공식 데이터 경로를 유지하고 Quosparr가 복호화하지 않으며 계정 내보내기에 포함하지 않습니다. 원래 구성은 계정을 가져오지 않고 따로 보존하며 브라우저나 다른 앱 데이터는 읽지 않습니다. 프로필과 Desktop 데이터는 같은 볼륨에 있어야 하고 중단 후 명시적으로 복원할 수 있습니다. API 계정은 제외되며 macOS, Windows, Linux 실기기 및 Desktop 버전 호환성 검증은 아직 완료하지 않았습니다.

**Grok**

- 공식 CLI chat-proxy 결제 API로 포함 할당량, Build / Imagine / Chat 분해, 주간 / 월간 주기, 선불 잔액, 온디맨드 사용량을 봅니다.
- 여러 OAuth / SuperGrok 계정을 격리 로그인, 현재 로그인 가져오기, 자격 증명 붙여넣기, 새로고침, 별칭, 정렬, 명시적 전환으로 관리합니다.
- 로컬 Grok CLI 로그의 최근 세션.

**세 공급자 공통**

- 설정의 “모델 가격”은 “고급” 바로 앞의 같은 그룹에 있습니다; models.dev 공개 카탈로그를 기본적으로 매시간 자동 동기화하며 끄거나 15분~7일의 기본·사용자 지정 간격을 선택할 수 있고, 변경은 즉시 저장되며 자동 동기화를 꺼도 수동 동기화할 수 있습니다
- 모델 가격은 백만 토큰당 USD 참고 요금이며 이미지·동영상 등 모든 모달리티 요금을 포함하지 않습니다; 검증된 컨텍스트 구간, 캐시 쓰기 TTL, Fast, 지역 요금을 유지하기 위해 카탈로그 참고 가격과 적용 가격이 다를 수 있으며 상세에 출처와 버전을 표시하고 아래 날짜가 있는 내장 표는 오프라인 기준으로 유지합니다；OpenAI, Anthropic, xAI의 원래 공급자 ID 항목과 내장 가격만 고정 Codex / Claude / Grok 탭에 표시하며 기본은 Codex이고 다른 공급자 옵션은 없습니다; 타사 중계와 다른 공급자의 동명 모델은 제외하며 models.dev는 집계된 공개 카탈로그로 공식 실시간 공급자 API가 아닙니다；탭 옆 동기화 설정 버튼으로 네이티브 대화상자를 열며 자동 동기화, 주기, 상세 상태는 세 공급자 공통으로 즉시 저장되고 창을 닫아도 동기화는 계속됩니다
- API 환산 비용은 과거 로컬 사용량을 현재 가격으로 다시 추정합니다; 합계·일별·프로젝트·모델·세션의 일부 비용을 표시하고 누락된 요금이나 규칙을 모두 나열하며, 가격 없음은 —, 명시적 무료는 $0으로 표시하고 구독 할당량과 reset credits 기준은 유지합니다
- 가격 동기화는 저장된 프록시 정책으로 공개 카탈로그만 다운로드하며 세션 로그를 업로드하지 않습니다; 오프라인에서는 내장 가격과 마지막으로 저장에 성공한 카탈로그를 유지하고 동기화 오류를 표시합니다
- 항상 보이는 `Codex | Claude | Grok` 탭. macOS 설정의 “메뉴 막대 항목”에서 할당량 막대, 카운트다운, 배터리(가로 또는 세로), 할당량 링(값을 링 안에 표시 가능), 진행 텍스트 다섯 가지 스타일을 미리 볼 수 있습니다. 메뉴 막대는 주 패널을 따르거나 세 공급자 중 원하는 공급자를 각각 따로 표시하며, 할당량 막대·카운트다운·배터리는 두 공급자를 한 항목에 위아래로 쌓을 수도 있습니다. 이미 저장된 “Codex + Grok”은 그 쌍을 유지합니다.
- 선택 사항인 항상 위 데스크톱 플로팅 창은 공급자를 최대 세 개까지 남은 사용량으로 보여 주며 캡슐, 카드, 링, 배터리 네 가지 스타일을 제공합니다. 클릭하면 주 패널이 열리고, 드래그로 옮기며, 오른쪽 클릭으로 더 많은 옵션을 봅니다. 불투명도, 위치 잠금, 전체 화면 앱 위 표시 여부를 설정할 수 있습니다.
- 오늘, 현재 주기, 이전 주기, 이번 달 또는 사용자 지정 범위의 API 환산 비용과 토큰 사용량.
- 연초부터의 토큰 사용량 차트(히트맵, 꺾은선, 막대 / 일간, 주간, 누적).
- Skills 페이지에서 공급자별 전역 skill의 출처, CLI 로드 여부, 함께 쓰는 CLI를 보여 줍니다. 검색, `SKILL.md` 보기, 폴더 열기, CLI 자체 설정에서 켜기 / 끄기(쓰기 전 백업), 다른 CLI로 복사 또는 이동(같은 이름은 덮어쓰지 않음), Quosparr Skill 휴지통으로 이동, 끊어진 링크, 중첩 폴더, 중복 이름 진단을 지원하며 대부분은 한 번에 고칠 수 있고 여러 skill을 선택해 함께 삭제하거나 고칠 수도 있으며, 언제나 위험을 먼저 보여 주고 확인을 받습니다. 설정의 Skill 관리는 공급자별 탭과 상태·유형 필터, 설치 출처·파일·frontmatter·GitHub 저장소나 홈페이지 링크를 담은 상세 페이지, 삭제한 skill을 복원하거나 시스템 휴지통으로 보내는 휴지통을 제공합니다.
- 설정의 세션 관리에서 공급자별 로컬 세션 전체를 볼 수 있습니다. 제목, 프로젝트, ID로 검색하고 프로젝트와 날짜 범위로 거를 수 있으며, 상세 페이지에서 모델, 토큰 내역, API 환산 비용, 관련 파일, 대화 미리보기를 확인합니다. 선택한 세션은 확인 후 하나씩 시스템 휴지통으로 옮기며 영구 삭제하지 않습니다. 공식 인덱스와 데이터베이스는 그대로 두고, 파생 통계는 남은 로그로 다시 계산합니다.
- 설정의 MCP 관리에서 공급자별 사용자 수준 전역 MCP 서버(Codex와 Grok은 `config.toml`, Claude는 `.claude.json`)를 보여 줍니다. 명령, 사이트, 구조는 민감 정보를 가린 요약으로만 표시하고 env와 headers 값은 보여 주지 않으며, 서버에 연결하거나 실행하지 않습니다. 여러 항목을 선택해 삭제할 수 있고, 삭제 전에 `~/.quosparr/backups`에 백업하며 다른 설정은 유지합니다. 변경 사항은 해당 CLI를 다시 시작하면 적용됩니다. Codex와 Grok 서버에는 켜기/끄기 스위치도 있으며, 같은 방식으로 먼저 백업한 뒤 저장합니다. Codex는 항목의 `enabled`를 쓰고, Grok은 공식 CLI와 같이 `disabled_mcp_servers`도 함께 갱신합니다. Claude Code는 MCP 활성화 상태를 프로젝트별로 기록하므로 Claude 서버에는 스위치가 없습니다. 프로젝트, 플러그인, 기업 관리 MCP는 표시하지 않습니다.
- 라이트, 다크, 시스템 모양과 English, 简体中文, 繁體中文, 한국어, 日本語, Русский, Français.
- 내장 업데이트와 macOS 14+ 데스크톱 위젯.

## 스크린샷

<table>
  <tr>
    <th colspan="2">메인 패널</th>
    <th colspan="2">API 환산 비용</th>
    <th colspan="2">구독 한도 추정</th>
  </tr>
  <tr>
    <td colspan="2" align="center" valign="top"><img src="docs/images/1-en.webp?v2" alt="Quosparr 메인 패널" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/3.webp?v1" alt="Quosparr API 환산 비용" width="240"></td>
    <td colspan="2" align="center" valign="top"><img src="docs/images/6.webp?v1" alt="Quosparr 구독 한도 추정" width="240"></td>
  </tr>
  <tr>
    <th colspan="3">공급자 계정</th>
    <th colspan="3">표시 설정</th>
  </tr>
  <tr>
    <td colspan="3" align="center" valign="top"><img src="docs/images/4.webp?v1" alt="Quosparr 공급자 계정" width="370"></td>
    <td colspan="3" align="center" valign="top"><img src="docs/images/5.webp?v1" alt="Quosparr 표시 설정" width="370"></td>
  </tr>
  <tr>
    <th colspan="6">Windows 설정 및 Grok 패널</th>
  </tr>
  <tr>
    <td colspan="6" align="center" valign="top"><img src="docs/images/2.webp?v1" alt="Quosparr Windows 설정 및 Grok 패널" width="780"></td>
  </tr>
</table>

스크린샷은 macOS와 Windows에서의 v1 인터페이스입니다.

## 설치

[GitHub Releases](https://github.com/Licoy/Quosparr/releases)에서 내려받고, 사용하는 시스템과 CPU에 맞는 패키지를 고르세요.

| 시스템 | 패키지 | 설치 방법 |
| --- | --- | --- |
| macOS 12+ (Apple Silicon / Intel) | `Quosparr-macos-arm64.dmg` / `Quosparr-macos-x86_64.dmg` 또는 같은 이름의 `.zip` | `Quosparr.app`을 `Applications`로 끌어다 놓습니다 |
| Windows 10 / 11 (amd64 / arm64) | `Quosparr.Setup.<version>.<arch>.exe` | `%LOCALAPPDATA%\Programs\Quosparr`에 사용자 단위로 설치하며 관리자 권한이 필요 없습니다 |
| Linux (amd64 / arm64) | `install.sh`가 있는 `quosparr-<version>-linux-<arch>.tar.gz` 또는 `quosparr_<version>_<arch>.deb` | tarball 옆에서 `sh install.sh`를 실행해 `~/.local`에 설치하거나, 패키지 관리자로 deb를 설치합니다 |

모든 패키지에는 해당 아키텍처용 SQLite 런타임이 들어 있어 처음 실행할 때 아무것도 내려받지 않습니다. 처음 실행하면 Quosparr는 기본적으로 로그인 시 실행을 켭니다. **제어판 → 일반 → 로그인 시 실행**에서 바꿀 수 있습니다.

Homebrew 설치는 아직 제공되지 않습니다. 당분간 DMG 또는 ZIP을 사용하세요.

이전에 CodexRunway를 사용했다면 [업그레이드 가이드](docs/upgrade-from-codexrunway_KO.md)를 참고하세요.

### macOS 보안 안내

macOS 빌드는 현재 ad-hoc signed이며 notarized되지 않았습니다. 개발자를 확인할 수 없다는 메시지가 나오면 `Quosparr.app`을 마우스 오른쪽 버튼으로 클릭해 열기를 선택하거나, 시스템 설정 → 개인 정보 보호 및 보안에서 그래도 열기를 클릭하세요.

앱이 손상되었다는 메시지가 나오면 보통 다운로드 격리 속성 때문입니다. 앱을 `Applications`로 옮긴 뒤 다음을 실행하세요.

```bash
xattr -dr com.apple.quarantine /Applications/Quosparr.app
```

### Linux 요구 사항

Linux에는 GTK3, AppIndicator, 트레이를 지원하는 데스크톱, 프록시 자격 증명을 저장할 수 있는 Secret Service 세션(예: GNOME Keyring 또는 KWallet)이 필요합니다. deb는 이 의존성을 선언하며 패키지 관리자를 통해 업데이트됩니다.

## 요구 사항

- 이 컴퓨터에 Codex가 설치되어 사용 중인 것을 권장합니다. Codex 데이터는 `CODEX_HOME`(기본값 `~/.codex`, Windows는 `%USERPROFILE%\.codex`)에서 가져옵니다.
- `file` 자격 증명 저장소만 지원합니다(Codex `config.toml`에 `cli_auth_credentials_store`가 없을 때의 기본값). `keyring`, `auto`, `ephemeral`은 지원되지 않는다고 표시되며 계정 가져오기와 전환이 차단됩니다.
- Claude 패널은 `CLAUDE_CONFIG_DIR`이 설정되어 있으면 해당 디렉터리, 아니면 `~/.claude`의 Claude Code 설정을 읽습니다. 구독 사용량에는 Claude.ai 로그인이 필요하며 API 키나 게이트웨이는 제공하지 않습니다. 브라우저 로그인은 CLI 설치나 사전 실행 없이 가능하며 명시적으로 전환하기 전에는 계정 라이브러리에만 저장합니다. 기존 CLI 로그인은 자동으로 추가되며 자격 증명 JSON 붙여넣기나 파일 가져오기도 가능합니다. 실제 브라우저 인증과 전환은 격리 계정에서 인수 검증해야 합니다.
- Grok 패널에는 [공식 Grok CLI](https://docs.x.ai/build/overview)가 필요합니다.

  ```bash
  curl -fsSL https://x.ai/cli/install.sh | bash
  ```

- Grok 계정과 할당량은 OAuth / SuperGrok 및 호환 레거시 세션만 지원합니다. `grok login --oauth`를 실행하거나, 앱에서 로그인하거나, 현재 로그인을 가져오거나, 자격 증명 JSON을 붙여넣으세요. API 키만 있는 로그인은 관리 계정으로 추가되지 않습니다. `GROK_HOME`이 설정되어 있으면 그 값을 쓰고, 아니면 `~/.grok`를 씁니다.

## 데스크톱 위젯

위젯은 macOS 14 이상이 필요하며 Windows와 Linux에는 없습니다. 할당량 개요, 토큰 추세, 핵심 지표, Codex 초기화 업데이트의 네 가지 위젯을 제공합니다. 각 위젯은 위젯 편집에서 Codex, Claude, Grok 또는 한 쌍(Codex + Claude, Codex + Grok, Grok + Claude)을 고를 수 있습니다. 이미 Codex + Grok으로 둔 위젯은 그 쌍을 유지합니다. 탭하면 해당 페이지가 열립니다.

스냅샷은 `~/.quosparr/widget-snapshot.json`에 `0600` 권한으로 저장되며 비밀이 아닌 파생 데이터만 담습니다. 이메일, 계정 ID, 토큰, 인증 JSON, 이벤트 원문은 없습니다. 주 앱만 쓰고 위젯은 읽기만 합니다.

## 네트워크 프록시

**제어판 → 일반 → 네트워크**에서 **시스템 설정 사용**, HTTP, SOCKS5 중 하나를 선택합니다. 사용자 지정 프록시는 사용자 이름과 비밀번호를 지원합니다. 이 설정은 앱이 시작한 Codex, Claude, Grok 요청, CLI 작업, 공개 상태 및 가격 피드, 소프트웨어 업데이트에 적용됩니다. 시스템 프록시는 변경되지 않으며, 브라우저 로그인 페이지와 이미 실행 중인 외부 CLI는 자체 설정을 유지합니다.

변경 사항은 **프록시 설정 저장**을 누른 뒤에만 적용됩니다. **연결 테스트**는 현재 초안으로 테스트 사이트(기본값 `https://github.com`, 변경 가능)에 접속하며 계정 자격 증명을 보내거나 초안을 저장하지 않습니다. 프록시 비밀번호는 macOS 키체인, Windows 자격 증명 관리자 또는 Linux Secret Service에 저장됩니다. 잘못된 설정, 사용할 수 없는 자격 증명, 사용자 지정 프록시 실패 시에도 직접 연결로 전환하지 않습니다.

## 개인정보

- Quosparr는 자체 데이터를 `~/.quosparr`에 보관하며 앱 캐시를 `~/.codex`에 쓰지 않습니다. 기본적으로 비밀이 아닌 파생 데이터만 저장합니다.
- 공급자 계정 자격 증명과 위에서 명시적으로 확인한 Desktop 프로필은 통제된 예외입니다. `~/.quosparr/accounts/<id>/auth.json`에 저장되며(디렉터리 `0700`, 파일 `0600`, 원자적 쓰기, Windows는 현재 사용자만 접근할 수 있는 ACL 사용) 계정 인덱스에는 토큰이 없습니다.
- 공식 `~/.codex/auth.json`은 OAuth 새로고침 때, 또는 사용자가 계정 전환을 확인했을 때만 씁니다. 현재가 아닌 계정을 새로고침하면 라이브러리 사본만 업데이트합니다.
- Claude 자격 증명은 macOS에서 `/usr/bin/security`로 키체인을 읽고, Windows와 Linux에서는 `<config>/.credentials.json`을 읽습니다. 관리 사본은 `~/.quosparr/accounts/claude-<stable-id>/auth.json`이며 `claudeAiOauth`만 담습니다. 인덱스 `claude-index.json`에는 토큰이 없습니다. 전환은 MCP 토큰을 유지하고 `.claude.json`의 `oauthAccount`만 바꿉니다. Quosparr는 현재 계정을 새로고침하지 않습니다.
- API 계정 키는 관리 사본(`~/.quosparr/accounts/<id>/auth.json` 또는 `claude-api-<id>/auth.json`)에만 저장되고, 인덱스에는 Base URL, 모델, 잔액 캐시만 남습니다. 잔액과 모델 요청은 입력한 사이트로만 프록시 설정을 따라 전송됩니다. 수정되는 `config.toml`과 `settings.json`은 먼저 `~/.quosparr/backups`에 백업되며, `settings.json`에 처음 기록할 때 키 순서가 정렬됩니다.
- Grok 자격 증명은 `$GROK_HOME/auth.json`에서 읽으며 관리 사본은 `~/.quosparr/accounts/grok-<stable-id>/auth.json`에 있습니다. Grok 전환은 OAuth / 호환 레거시 범위만 바꾸고 API 키와 알 수 없는 범위는 유지합니다. 새 세션에만 보장되며, 실행 중인 Grok 프로세스는 종료되지 않고 이전 계정을 다시 쓸 수 있으므로 앱이 전환 전에 경고합니다.
- 세션 복구는 `session_index.jsonl`만 다루며 먼저 백업합니다. 세션 로그는 세션 관리에서 직접 선택하고 확인한 경우에만 시스템 휴지통으로 옮기며, 옮기기 전에 파일이 바뀌지 않았는지 하나씩 다시 확인합니다. 영구 삭제하지 않고 공식 인덱스나 데이터베이스도 고쳐 쓰지 않습니다.
- Skill과 MCP의 켜기 / 끄기, 복구, 삭제는 확인을 받은 뒤에만 실행합니다. 구성 파일은 쓰기 전에 `~/.quosparr/backups`에 백업하고, 삭제한 skill은 먼저 복원 가능한 Quosparr Skill 휴지통으로 가며, MCP 목록에는 키나 env, headers 값을 표시하지 않습니다.
- API 환산 비용은 로컬 세션 로그에서 계산합니다. 세션 내용은 업로드하지 않습니다.
- 토큰과 API 키는 로그, 자가 진단 출력, 이슈 템플릿에 쓰지 않습니다.
- Codex 초기화 업데이트와 업데이트 확인은 공개 데이터만 가져오며 계정, 토큰, 세션 내용은 보내지 않습니다.

## 데이터 출처

- **Codex 할당량 / reset credits / 공식 사용량**: 로컬 자격 증명을 사용하는 공식 ChatGPT / Codex 백엔드 API. 공식 사용량은 현재 계정에 속하며 지연되거나 수정될 수 있습니다.
- **주간 할당량 추정**: 비공식이며 주간 사용률과 일별 Credits로 외삽합니다(1000 Credits ≈ $40, 버전 `credits-usd-2026-08-26`). 파생 Credits와 사용률만 `quota-estimate-history.json`에 저장합니다.
- **Codex API 환산 비용**: 내장 가격표 `openai-builtin-2026-09-27`로 계산한 로컬 세션 로그 기반 비용이며, 지원되는 현재 요금은 models.dev에서 가져오고 검증된 내장 규칙을 유지합니다. 알 수 없는 모델은 정확한 비용으로 보고하지 않습니다.
- **Claude Code 구독 사용량**: 문서화되지 않은 OAuth usage 엔드포인트이며, 현재 Claude Code 구독만 나타냅니다. 공식 결제 API로 쓰지 않습니다. 브라우저 쿠키나 Console API 청구서는 읽지 않습니다. Desktop 프로필은 위의 별도로 명시적 확인을 받은 기능에서만 관리합니다.
- **Claude API 환산 비용**: Claude Code 로컬 세션 로그와 가격표 `anthropic-builtin-2026-10-14`로 계산합니다. 같은 답변은 메시지 ID로 한 번만 셉니다. 캐시 쓰기는 5분과 1시간으로 나누고 Fast 요금은 따로 둡니다. 알 수 없는 모델은 정확한 비용을 보고하지 않습니다. 일 경계는 로컬 시간대입니다.
- **Grok 할당량**: 공식 CLI chat-proxy `/v1/billing?format=credits` 엔드포인트만 사용합니다. 브라우저 쿠키, 로컬 로그에서 추정한 할당량, API 결제는 섞지 않습니다.
- **Grok API 환산 비용**: 로컬 `turn_completed` 사용량을 `xai-builtin-2026-09-27`로 계산하며 입력, 캐시, 출력, 긴 컨텍스트 요금을 포함합니다. `costUsdTicks`는 구독 회계이므로 API 환산 비용으로 쓰지 않습니다.
- **Codex 초기화 업데이트**: [didcodexreset.com/api/status.json](https://didcodexreset.com/api/status.json)의 공개 피드. 비공식이며 참고용이고 지연될 수 있습니다.
- 차트의 「공식 통계(모든 기기)」는 현재 계정에 속하고, 「로컬 로그(모든 세션)」는 이 컴퓨터를 스캔하므로 여러 계정에 걸칠 수 있습니다. 둘은 서로의 부분집합이 아니므로 빼면 안 되며, 두 Codex 일별 값은 모두 UTC 날짜를 씁니다.

## 개발

메인 앱은 Go로 작성되었으며 [mygo](https://github.com/egoist/mygo) 네이티브 UI를 씁니다. Go와 mygo 버전은 `go.mod`에 고정되어 있습니다. macOS 위젯 확장은 Swift로 유지됩니다.

```bash
go test ./...
go vet ./...
go build ./...
```

실제 계정과 설정을 건드리지 않도록 임시 HOME과 별도의 환경설정 도메인으로 실행하세요.

```bash
QUOSPARR_TEST_HOME="$(mktemp -d)"
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run .
HOME="$QUOSPARR_TEST_HOME" QUOSPARR_PREFS_DOMAIN=com.quosparr.app.v1-test go run . --self-check
```

자가 진단은 네트워크 요청을 하지 않고 자격 증명도 읽지 않습니다. 현재는 버전, 플랫폼, 자체 디렉터리 권한, 잠금, Codex auth와 세션 인덱스 존재 여부만 확인하며, 나머지 진단은 `not ported yet`로 표시됩니다.

macOS 개발 패키지를 빌드합니다(업데이터 비활성화, 선택적으로 `ARCH=arm64` 또는 `ARCH=x86_64`).

```bash
V1_DEVELOPMENT_PACKAGE=1 bash Scripts/package-v1-app.sh
```

Windows / Linux 패키징과 릴리스 점검은 [docs/v1-release.md](docs/v1-release.md)를, 기여 안내는 [CONTRIBUTORS.md](CONTRIBUTORS.md)를 보세요.

## 커뮤니티

- [LinuxDO](https://linux.do/)

## 라이선스

이 프로젝트는 저장소의 [LICENSE](LICENSE)를 따릅니다.
