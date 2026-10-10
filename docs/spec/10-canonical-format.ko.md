<!-- doc-id: spec-canonical-format -->
<!-- source-sha256: 751697d78e7ee20d359edaf9fc9a798478c6726291b21371dec22148a524d004 -->
# Canonical format

[English](10-canonical-format.md)

이 spec은 [document model](20-document-model.md)이 이름 붙인 저장 방식들의 byte 형식을 고정한다.
idempotency 명제, 문서 순서, list와 table 문법, 섹션, encoding을 정한다. renderer가 소유하는 것은
모두 여기 적고, 나머지는 보존되는 payload다.

## Idempotency

- **P1 (canonical identity).** canonical 입력 `s`에 대해 `emit(parse(s)) == s`, byte 단위로 같다.
- **P2 (emission stability).** 유효한 모든 입력 `d`에 대해 `emit(parse(emit(d))) == emit(d)`.
- P1과 P2가 단일 byte 동일성 주장을 대신한다: Windows 편집기가 CRLF로 저장한 문서는 유효한 입력이고
  P2는 여전히 성립한다. P1의 복원은 migration이지 논리적 쓰기의 부작용이 아니다.

## Skeleton과 보존 경계

renderer는 정확히 **skeleton**을 소유한다: header comment, frontmatter 블록, H1, 블록·상세 섹션의
heading 줄, 행의 list 줄과 table 줄, skeleton 요소 사이의 빈 줄. `markdown` column의 값 — body와
column 섹션 — 은 payload다: renderer는 이를 행과 함께 옮기고 그 byte를 고쳐 쓰지 않는다.

- canonical이 아닌 입력(BOM, CRLF, canonical이 아닌 형태의 skeleton 줄, 저장 순서를 벗어난 행)은
  `format.noncanonical` 진단이다. `flowmark canon`은 문서를 canonical 형식으로 고쳐 쓰는 일회성
  migration transaction이고, 논리적 쓰기는 부작용으로 정규화하지 않는다.

## 문서 순서

- **`files` 행**은 차례로: header comment(`<!-- doc-id: <key> -->`가 먼저, 그다음 provenance
  comment), frontmatter 블록, H1 제목, body, 행 scope의 level-2 섹션.
- **`document` file** `<table>.md`는: `<!-- doc-id: <table> -->`, `# <table>`, H1 바로 아래 행들의
  list나 table, level 2의 상세 섹션.
- **블록 섹션**은 부모 행 scope 안에서 자식 table 이름을 가진 level b heading이다. heading 뒤에 list나
  table이 오고, 그 행들의 상세 섹션이 level b + 1로 뒤따른다.
- **상세 섹션**은 heading `<h> <key>` — key만, 제목 사본은 없다 — 와 그 뒤의 행 body, 그리고 level
  h + 1의 행 scope 섹션이다. 블록의 어떤 행도 가리키지 않는 key의 상세 섹션은 `format.block`이다.
  중첩된 list 행의 상세 섹션은 평평한 형제다. 상세 섹션 없는 행은 빈 body를 갖는다.
- layout이 H6를 넘는 heading을 필요로 하면 catalog를 읽을 때 정해지는 schema 오류다.

## 섹션과 그 값

- level L 행 scope의 body와 모든 `markdown` column 값은 fenced block 밖에서만 세는(CommonMark block
  구조) 다음 level ≤ L heading에서 끝난다. 값은 L보다 깊은 heading만 담으며, fence 밖에 level ≤ L
  heading 줄을 가진 값의 쓰기는 `CONSTRAINT`다.
- level L 행 scope에서 nullable `markdown` column과 자식 블록은 각각 column이나 자식 table 이름을
  가진 level L heading이다. 없는 섹션은 NULL, 비어 있는 섹션은 빈 텍스트다. body는 NULL이 아니다.
  선언되지 않은 level L heading은 `format.section`이다.
- 자식 블록 섹션은 블록이 행을 하나 이상 가질 때만 쓴다. 자식 행이 없는 부모 행은 블록 heading을
  갖지 않으며, 빈 블록 heading은 `format.noncanonical`이다.

## 저장되는 scalar column

table의 저장되는 scalar column은 column에서 key, `title`, `body`, `markdown` column, `list`의 상태
column, 배치 foreign key, `list`의 `parent`, block이나 document 저장의 `order` column을 뺀 것이다.
key, 제목, 상태, 계층은 각자 자리를 가진다. 저장되는 scalar column은 list field, table cell, frontmatter
key다. null이 아닌 저장 scalar column은 모두 쓴다 — 엔진은 insert에서 default를 명시적으로 쓴다.

## List 행

- list 행은 `- [<glyph>] <key> <title>[ <field>…]`이고, table에 상태 column이 없으면
  `- <key> <title>[ <field>…]`이다. glyph는 행 상태의 `checkbox` glyph다.
- field는 오른쪽부터 parse한다: token `<선언된 column>:<value>`이고, value는 공백, `"`, `\` 없는 bare
  token이거나 `\"`와 `\\`를 escape로 갖는 `"…"`다. 저장되는 scalar column만 field다. 없는 field는 NULL,
  `col:""`은 빈 텍스트이며, non-null field가 빠진 행은 `format.block`이다.
- `<선언된 column>:`과 맞는 제목 token은 `<column>\:`로 쓴다. 제목은 key와 첫 field 사이의 텍스트다.
- 자식 행은 부모보다 두 칸 더 들여 쓴다. 들여쓰기가 `parent` column이며 다른 것이 이를 쓰지 않는다.
  배치 foreign key와 `parent`는 field로 쓰지 않으며, 쓰면 `format.block`이다.

## Table 행

- table의 header 행은 저장되는 scalar column을 이름 붙인다. 저장되는 모든 scalar column은 header를
  가지며, 어떤 column도 이름 붙이지 않는 header는 `format.block`이다. cell은 `|`를 `\|`로 쓴다.
- 빈 cell은 NULL이고, 빈 텍스트는 `""`로 쓰며, 모든 non-null 저장 scalar column은 값을 갖는다. `"`로
  시작하는 cell 값은 list field와 같이 `"…"` 안에 `\"`와 `\\`를 escape로 써서 쓴다.

## 한 줄 자리

list 제목, list field, table cell은 한 줄이다: 그 자리에 줄바꿈을 가진 값은 쓰기에서 `CONSTRAINT`다.
list나 table 저장의 key는 공백, `#`, `|`, 줄바꿈을 갖지 않는다.

## Frontmatter profile

- YAML **1.2 core schema**, 값 영역은 block style이며 anchor, alias, tag는 거절한다. comment는 v1에서
  거절한다(comment를 보존하는 round-trip emitter는 요구가 이름 붙일 때까지 명시된 비목표다).
- 각 key는 `files` table의 저장되는 scalar column이며, 모르는 key는 `format.frontmatter`다. 없는 key는
  NULL이며, 모든 non-null 저장 scalar column은 자기 key를 갖고 빠지면 `format.frontmatter`다.
- type별 값 문법: 모든 값은 dbspec canonical literal 형식을 쓰고 `bytes`는 base64다.

## Encoding

BOM 없는 UTF-8. canonical 줄 끝은 LF이고, 입력은 CRLF를 유효하다고 받으며(P2) canonical이 아니라고
보고한다(P1). key는 NFC이고, 그 밖의 payload byte는 있는 그대로 보존한다.

## 진단

위의 모든 규칙은 `{file, line, column, rule, message}`를 보고하며 규칙 이름은
`format.noncanonical`, `format.header`, `format.frontmatter`, `format.section`, `format.block`,
`format.encoding`이다.
