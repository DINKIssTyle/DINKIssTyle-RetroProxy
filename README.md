# DKST RetroProxy
> **Modern websites for legacy browsers**

<p align="center">
  <img src="img/DKST_RetroProxy_CNN_NC3_Image.png">
</p>

<p align="center">
  <a href="README.EN.md">English</a> | <strong>[ Korean ]</strong>
</p>


**DKST RetroProxy**는 최신 웹사이트를 구형 브라우저(Netscape Navigator, Internet Explorer 3~5 등)에서도 볼 수 있도록 변환해주는 프록시 서버 애플리케이션입니다.



<p align="center">
  <img src="img/DKST_RetroProxy_Win_Main.png">
</p>



최신 웹 기술(HTML5, CSS3, JS)로 만들어진 사이트를 Headless Browser 기술을 사용하여 렌더링한 뒤, 구형 브라우저가 이해할 수 있는 단순한 HTML 3.2/4.01 구조나 이미지 맵으로 변환하여 전달합니다.

---

## ✨ 주요 기능 (Features)

### 1. 렌더링 모드 (Rendering Modes)

<p align="center">
  <img src="img/DKST_RetroProxy_News_NC3_Html401.png">
</p>


*   **HTML 3.2 / 4.01 모드**: 최신 웹 페이지의 DOM을 분석하여 테이블 기반 레이아웃의 단순한 HTML로 재구성합니다.
*   **Text Only 모드**: 이미지와 복잡한 요소를 제거하고 텍스트와 링크만 남겨 텍스트 기반 브라우저(Lynx 등)나 초저사양 환경에 최적화합니다.
*   **Image Map 모드**: 페이지 전체를 이미지로 캡처한 뒤, 클라이언트 측 이미지 맵(Image Map)을 생성하여 링크 클릭이 가능하도록 합니다. 간단한 텍스트 입력도 지원하여 검색창 등을 사용할 수 있습니다. 웹 표준을 지원하지 않는 아주 오래된 브라우저에서도 시각적으로 완벽한 페이지를 볼 수 있습니다. (대용량 페이지도 스크롤 캡처 지원)

### 2. 호환성 지원 (Compatibility)
*   **HTTPS → HTTP 다운그레이드**: SSL/TLS를 지원하지 않는 레거시 브라우저를 위해 모든 통신을 HTTP로 변환합니다.
*   **인코딩 변환**: UTF-8을 지원하지 않는 브라우저를 위해 EUC-KR, CP949, Shift_JIS, ISO-8859-1 등으로 실시간 인코딩 변환을 지원합니다.
*   **이미지 최적화**: WebP 등 최신 이미지를 JPEG/PNG로 자동 변환합니다.

### 3. 관리 및 디버그 (Management & Debug)

<p align="center">
  <img src="img/DKST_RetroProxy_In_web_Settings.png">
</p>

*   **원격 관리 페이지**: 클라이언트 브라우저에서 `http://server`, `http://setting`, `http://settings`에 접속하여 설정을 변경하거나 서버를 종료할 수 있습니다.
*   **디버그 툴바**: 변환된 페이지 상단에 원본 URL 확인 및 인코딩/모드 변경 툴바를 제공합니다.

---

## 🏗️ 빌드 방법 (Build)

이 프로젝트는 Go와 Wails 프레임워크를 기반으로 합니다. 포함된 빌드 스크립트를 사용하여 간편하게 빌드할 수 있습니다.

### 필수 요건 (Prerequisites)
*   **Go**: 1.18+
*   **Node.js**: 14+
*   **Wails CLI**: (스크립트가 자동으로 설치합니다)

### 빌드 실행 (Execute Build)
프로젝트 루트 디렉토리에서 운영체제에 맞는 스크립트를 실행하세요.

#### Windows
`build.bat` 파일을 더블 클릭하거나 터미널에서 실행합니다.
```cmd
build.bat
```

#### macOS / Linux
`build.sh` 스크립트에 실행 권한을 주고 실행합니다.
```bash
chmod +x build.sh
./build.sh
```

빌드가 완료되면 `build/bin` 디렉토리에 실행 파일이 생성됩니다.

---

## 🚀 사용 방법 (Usage)

### 1. 서버 실행
1. `RetroProxy.exe`를 실행합니다.
2. 포트 번호(기본값 8080)를 확인하고 **Start** 버튼을 누릅니다.
   * *Status가 "Running"으로 바뀌면 정상 작동 중입니다.*

### 2. 클라이언트(레거시 브라우저) 설정
구형 PC의 브라우저 설정에서 **HTTP 프록시**를 설정합니다.

*   **Address (주소)**: DKST RetroProxy가 실행 중인 PC의 IP 주소 (예: `192.168.0.10`)
*   **Port (포트)**: `8080` (또는 설정한 포트)

### 3. 웹 서핑
브라우저 주소창에 가고 싶은 웹사이트 주소(예: `www.google.com`, `news.naver.com`)를 입력합니다.
DKST RetroProxy가 자동으로 페이지를 변환하여 보여줍니다.

---

## 🛠️ 관리 페이지 (Server Management)

레거시 브라우저에서 프록시를 통해 다음 주소로 접속하면 설정 메뉴가 나타납니다.

*   **URL**: `http://server`, `http://setting`, `http://settings`

**기능:**
*   **Encoding**: 웹페이지 문자셋 강제 설정 (Auto, UTF-8, EUC-KR 등)
*   **HTML Version**: 변환 타겟 버전 설정 (3.2 / 4.01)
*   **Render Mode**: HTML 모드 / 텍스트 모드 / 이미지 맵 모드 변경
*   **Debug Mode**: 디버그 툴바 표시 여부 설정
*   **Server Control**: 서버 재시작(Restart) 및 종료(Shutdown)

---

## ⚠️ 주의 사항
*   이 프로그램은 보안 목적으로 설계되지 않았습니다. 중요한 개인정보(로그인, 결제 등)가 포함된 사용은 권장하지 않습니다.
*   매우 복잡한 SPA(Single Page Application) 사이트는 이미지 맵 모드를 사용하는 것이 좋습니다.

---
**(C) 2025 DINKI'ssTyle**
