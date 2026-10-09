<!-- doc-id: spec-canonical-format -->
<!-- source-sha256: dcf3e1681a2b1bf6e87ef489b891f299bfb1039fe4e7b71be5a24859136c6095 -->
# Canonical format

[English](10-canonical-format.md)

이 명세는 하나의 문서의 byte 형식을 확정한다. grammar subset, 멱등성 명제, ordering 규칙,
encoding을 서술한다. renderer가 소유하는 모든 것은 여기에 있고, 나머지는 보존되는 payload다.

## 멱등성

- **P1 (canonical 동일성).** canonical 입력 `s`에 대해: `emit(parse(s)) == s`, byte 단위로.
- **P2 (emit 안정성).** 임의의 valid 입력 `d`에 대해: `emit(parse(emit(d))) == emit(d)`.
- P1과 P2가 단일 byte-identity 주장을 대체한다: Windows editor가 CRLF로 저장한 문서도 valid
  입력이고 P2는 여전히 성립한다; P1을 회복하는 것은 migration이지 논리적 write의 부수 효과가
  아니다.

## Skeleton과 보존 boundary

renderer가 소유하는 것은 정확히 **skeleton**이다: stamp를 찍는 header comment(`doc-id`,
`source-sha256`, provenance), frontmatter block, heading line, 정렬 대상인 item·table line,
skeleton 요소 사이의 빈 줄. 그 외 모든 byte — 산문, fenced block, inline 서식, 소유하지 않는
marker를 제외한 HTML comment — 는 **보존 payload**다: renderer는 skeleton 줄을 옮기고 묶고
재정렬하되 payload byte는 결코 다시 쓰지 않는다.

- 비-canonical 입력(BOM, CRLF, 비-canonical 형태의 skeleton 줄)은 `format.noncanonical`
  diagnostic이다. `flowmark canon`은 문서를 canonical 형식으로 다시 쓰는 일회성 migration
  transaction이고, 논리적 write는 결코 정규화를 부수 효과로 하지 않는다.

## 문서 grammar

문서는 순서대로: header comment; H1 제목; preamble; YAML frontmatter block; 그리고 H2 section과
구조화 block. strict parser가 정확히 받아들이는 것은:

- **Header comment**: `<!-- doc-id: <id> -->` 정확히 한 번, 맨 앞; twin에는
  `<!-- source-sha256: … -->`; adapter 소유 문서에는 provenance comment.
- **Preamble**: H1과 첫 H2 사이의 text는 예약된 long-text column `preamble`에 mapping된다.
- **H2 section**: 선언된 long-text column마다 H1 heading 하나; 없는 section은 NULL, 있으나
  비어 있으면 빈 text. 선언되지 않은 column의 heading은 diagnostic이다.
- **구조화 block**은 [문서 모델](20-document-model.ko.md)의 child-table 배치다.

## Frontmatter profile

- YAML **1.2 core schema**, 값 영역은 block style; anchor·alias·tag는 거부; v1에서 comment는
  거부한다 (comment를 보존하는 round-trip emitter는 요구사항이 이름을 붙이기 전까지 명시된
  비목표다).
- key 문법은 dbspec name 규칙을 따르고 NFC로 정규화한다; 알 수 없는 key는 diagnostic다.
- type별 값 문법: decimal·datetime·uuid literal은 dbspec canonical 형식; `bytes`는 base64;
  줄바꿈을 가질 수 있는 text 값은 markdown-table cell에 둘 수 없다.

## Task ID와 ordering

- **Grammar**: `T<n>(.<n>)*(-<n>)?` — leading zero와 suffix `-0` 금지; 동등성은 canonical 형식의
  문자열 동등성이다. suffix는 신원의 일부이고 renumber 때 base와 함께 변경된다.
- **Ordering은 type-directed**: ID-typed key는 segment별 숫자 비교 — `T8.0.9`는 `T8.0.10` 앞에
  온다 — suffix는 base path 뒤에서 숫자 비교, suffix 항목의 하위는 suffix를 마지막 path
  segment에 결합한다(`T8.0.14-1.3`은 `T8.0.14-1`의 하위). text는 codepoint, 복합 key는
  component type별 사전식. `group_count`는 NULL을 같은 group으로 처리한다.
- **참조는 구조적이다**: 참조는 산문 속의 task-ID token이고 산문은 fenced block과 inline code
  span을 제외한다 — grammar 예시는 참조가 아니다. 참조를 다시 쓰는 codemod가 payload 보존의
  명시된 예외다.

## Encoding

BOM 없는 UTF-8; LF가 canonical 줄 끝; 입력은 CRLF를 valid로 받아(P2) 비-canonical으로
보고한다(P1); ID와 key는 NFC; payload byte는 그 외에는 있는 그대로 보존한다.

## Diagnostic

위 모든 규칙은 `{file, line, column, rule, message}`로 보고하고 rule 이름은
`format.noncanonical`, `format.header`, `format.frontmatter`, `format.section`,
`format.block`, `format.encoding`이다.
