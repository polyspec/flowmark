<!-- doc-id: spec-canonical-format -->
<!-- source-sha256: 8331ffa5ed8a2f74fe89a3c556ef949f502f732dde347c4c504c761fc8d65ff2 -->
# Canonical format

[English](10-canonical-format.md)

이 spec은 [document model](20-document-model.md)이 이름 붙인 corpus 문서의 byte 형식을 고정한다.
idempotency 명제, readable 문서, skeleton, 문서 순서, 상세 heading과 anchor, 섹션, 읽을 수 있는 값
형식, 한 줄 자리, field list, list와 table 문법, link, encoding을 정한다. renderer가 소유하는 것은
모두 여기 적고, 나머지는 보존되는 payload다.

## Idempotency

- **P1 (canonical identity).** canonical 입력 `s`에 대해 `emit(parse(s)) == s`, byte 단위로 같다.
- **P2 (emission stability).** 유효한 모든 입력 `d`에 대해 `emit(parse(emit(d))) == emit(d)`.
- P1과 P2가 단일 byte 동일성 주장을 대신한다: Windows 편집기가 CRLF로 저장한 문서는 유효한 입력이고
  P2는 여전히 성립한다. P1의 복원은 migration이지 논리적 쓰기의 부작용이 아니다.

## Readable 문서

- corpus 문서는 이름이 `.flowmark.md`로 끝나는 file이다. 기준은 [document model](20-document-model.md)이
  정한다.
- corpus 문서는 완전한 markdown 문서다. flowmark를 모르는 독자도 있는 그대로 읽는다: comment,
  marker, frontmatter, 숨은 field를 갖지 않는다.
- 구조는 평범한 markdown이다: heading, list, table, field list. 모든 관계는 GitHub식 heading slug를 쓰는
  markdown viewer(GitHub, GitLab, VS Code, Pandoc)에서 따라갈 수 있는 link다.
- 근거: 문서는 database의 유일한 저장이므로 읽을 수 있는 문서로 남아야 한다. 기계만 필요로 하는 것은
  catalog(`dbspec/`)나 `.flowmark/`에 속한다.

## Skeleton과 보존 경계

renderer는 정확히 **skeleton**을 소유한다: `files` 행의 H1 줄과 field list, block·상세·markdown column
섹션의 heading 줄, 행의 list 줄과 table 줄, skeleton 요소 사이의 빈 줄. `markdown` column의 값 — body와
column 섹션 — 은 payload다: renderer는 이를 행과 함께 옮기고 그 byte를 고쳐 쓰지 않는다.

- canonical이 아닌 입력(BOM, CRLF, canonical이 아닌 형태의 skeleton 줄, 저장 순서를 벗어난 행)은
  `format.noncanonical` 진단이다. `flowmark canon`은 문서를 canonical 형식으로 고쳐 쓰는 일회성
  migration transaction이고, 논리적 쓰기는 부작용으로 정규화하지 않는다.

## 문서 순서

- **`files` 행**은 `<table>/<key>.flowmark.md` file이며, 차례로: H1 줄 `# <title>`, 저장되는 scalar
  값의 field list 뒤 빈 줄, body, 행 scope의 level-2 섹션을 가진다.
- **`document` file** `<table>.flowmark.md`는: H1 줄 `# <table>`, H1 바로 아래 행들의 list나 table,
  level 2의 상세 섹션을 가진다.
- **상세 섹션**은 heading `<h> <table> row <key>` — table 이름, 공백 하나, `row`, 공백 하나, key — 이며 제목 사본은 없다.
  그 뒤에 행의 body가 오고, level h + 1의 행 scope 섹션이 뒤따른다. 상세 행의 scope는 level h + 1이다.
  중첩된 list 행의 상세 섹션은 평평한 형제다.
- **블록 섹션**은 부모 행 scope 안에서 자식 table 이름을 가진 level b heading이다. heading 뒤에 list나
  table이 오고, 그 행들의 상세 섹션이 level b + 1로 뒤따른다.
- 행 scope의 섹션은 고정된 순서를 따른다: nullable `markdown` column은 column 순서대로, 그다음 자식 블록은 table 이름 순서대로(dbspec의 정준 순서). 상세 섹션은 행의 저장 순서를 따른다.
- block이나 document의 list나 table 뒤의 내용은, 그 행들의 상세 섹션을 제외하고 모두 `format.block`이다. 이 내용은 block 섹션에서는 level b 이하의 다음 heading까지, document 파일에서는 파일 끝까지다.
- null이 아닌 저장 scalar가 없는 `files` 행은 field list가 없으며, body는 H1 뒤 빈 줄 하나 다음에 온다.
- 행은 body가 비어 있지 않거나, nullable `markdown` column이 NULL이 아니거나, 자식 블록 행을 가질 때,
  정확히 그때 상세 섹션을 갖는다. 이 중 어느 것도 없는 상세 섹션은 `format.noncanonical`이다. 상세
  섹션 없는 행은 빈 텍스트를 body로 갖는다.
- 블록의 어떤 행도 가리키지 않는 key의 상세 섹션은 `format.block`이다.
- layout이 H6를 넘는 heading을 필요로 하면 catalog를 읽을 때 정해지는 schema 오류다.

## 상세 heading과 anchor

- 상세 섹션의 anchor는 `<table>-row-<소문자 key>`다: heading `## items row T2`의 anchor는
  `items-row-t2`다. table은 anchor 텍스트에서 첫 `-` 앞이며, table 이름에는 `-`가 없다.
- slug는 GitHub의 것이다: heading의 렌더된 inline 텍스트를 소문자로 하고, 문자, 숫자, 공백, `-`, `_` 외의 모든
  문자를 지우며, 각 공백을 `-`로 쓴다. 엔진은 file 안의 모든 heading — skeleton과 payload 모두 — 의 slug를
  문서 순서대로 계산한다.
- 상세 anchor는 GitHub slug의 두 중복 처리 모두에서 비어 있어야 한다: (a) github-slugger는 반복되는 slug가
  아직 차지되지 않은 첫 접미사 `-1`, `-2`, …를 받는다. (b) github.com은 반복되는 기본 slug에 확인 없이
  counter 접미사를 붙인다. level b 블록 또는 markdown column 섹션 heading은 반복되든 아니든 `<name>` 또는
  `<name>-<digits>`로 slug가 되며 `<table>-row-<key>` anchor와 결코 같지 않으므로, 상세 anchor를 가져갈 수
  있는 것은 payload heading과 `files` 행의 H1 제목뿐이다.
- 다른 heading이 anchor를 가져갔거나 slug가 anchor와 다른 상세 heading은 쓰기에서 `CONSTRAINT`이며, 그
  heading을 밝히고 충돌을 일으키는 쓰기 — 제목 갱신, payload 쓰기, 행 삽입 — 를 거절한다. 읽기에서는
  `format.block`이다.
- 공백이 더 있거나 `row`의 대소문자가 다른 상세 heading은 `format.noncanonical`이며, `flowmark canon`이 고친다.

## 섹션과 그 값

- level L 행 scope의 body와 모든 `markdown` column 값은 fenced block 밖에서만 세는(CommonMark block
  구조) 다음 level ≤ L heading에서 끝난다. 값은 L보다 깊은 heading만 담으며, fence 밖에 level ≤ L
  heading 줄을 가진 값의 쓰기는 `CONSTRAINT`다.
- level L 행 scope에서 nullable `markdown` column과 자식 블록은 각각 column이나 자식 table 이름을
  가진 level L heading이다. 없는 섹션은 NULL, 비어 있는 섹션은 빈 텍스트다. body는 NULL이 아니다.
  선언되지 않은 level L heading은 `format.section`이다.
- 자식 블록 섹션은 블록이 행을 하나 이상 가질 때만 쓴다 — 빈 블록과 없는 블록은 모두 행이 0개라는 뜻이므로, 정식 형식은 하나뿐이다(P1). 자식 행이 없는 부모 행은 블록 heading을
  갖지 않으며, 빈 블록 heading은 `format.noncanonical`이다.
- `markdown` column은 모든 저장에서 언제나 행 scope의 섹션이다 — field나 cell이 아니다.

## 저장되는 scalar column

table의 저장되는 scalar column은 column에서 key, `list`나 `files` 저장의 `title`, `body`, `markdown` column, `list`의 상태
column, 배치 foreign key, `list`의 `parent`, block이나 document 저장의 `order` column을 뺀 것이다.
key, 제목, 상태, 계층은 각자 자리를 가진다. 저장되는 scalar column은 `files` 행의 field list 항목,
list 행의 field, table의 cell이다. null이 아닌 저장 scalar column은 모두 쓴다 — 엔진은 insert에서
default를 명시적으로 쓴다.

## 읽을 수 있는 값 형식

scalar 값은 dbspec literal이 아니라 읽을 수 있는 형식으로 쓴다. type은 언제나 catalog에서 온다.

- text는 있는 그대로 쓴다. 정수, decimal, float는 canonical 수 형식으로, `true`와 `false`는 그대로 쓴다.
- date는 `YYYY-MM-DD`, datetime은 `YYYY-MM-DD HH:MM:SS[.f…]`, time은 `HH:MM:SS[.f…]`다. datetime과
  time은 소수 자릿수가 column의 precision과 정확히 같다.
- uuid는 소문자다. bytes는 padding이 있는 base64로 쓴다.
- foreign key는 link다([외래 key의 link](#외래-key의-link) 참조).
- 엔진은 수와 `true`/`false`를 orm의 `dbspec.CanonicalLiteral(t, v)`로, date, time, datetime, uuid를
  `CanonicalLiteral(t, "'" + v + "'")`로 읽고, 결과에서 따옴표를 뗀다.
- `now`는 값으로 거절한다. text는 column의 길이로 검사하며, 한 줄 자리에서는 다음 절의 규칙으로도 검사한다.
- canonical 형식이 쓴 형식과 다른 값은 `format.noncanonical`이다.

## List 행

- list 행은 `- [<glyph>] <key> <title>[ <field>…]`이고, table에 상태 column이 없으면
  `- <key> <title>[ <field>…]`이다. glyph는 행 상태의 `checkbox` glyph이며 정확히 한 글자다.
- list glyph가 ` `와 `x`가 아니면 흔한 viewer에서 대괄호가 든 글자로 그대로 보인다. 상태 이름은
  catalog의 것이고, `flowmark.dbs`의 glyph는 그 관례다.
- field는 오른쪽부터 parse한다: token `<선언된 column>:<value>`이고, value는 공백, `"`, `\` 없는 bare
  token이거나 `\"`와 `\\`를 escape로 갖는 `"…"`다. field에는 sigil이 없다. 저장되는 scalar column만
  field다.
- 없는 field는 NULL, `col:""`은 빈 텍스트다. 모든 non-null column은 쓰며, non-null field가 빠진 행은
  `format.block`이다.
- `<선언된 column>:`과 맞는 제목 token은 `<column>\:`로 쓴다. 제목은 key와 첫 field 사이의 텍스트다.
- 자식 행은 부모보다 두 칸(CommonMark list 항목 내용 들여쓰기) 더 들여 쓴다. 들여쓰기가 `parent` column이며 다른 것이 이를 쓰지 않는다.
- 배치 foreign key와 `parent`는 block list 행의 field로 쓰지 않으며, 쓰면 `format.block`이다.

## Table 행

- table의 header 행은 저장되는 scalar column을 이름 붙인다. 저장되는 모든 scalar column은 header를
  가지며, 어떤 column도 이름 붙이지 않는 header는 `format.block`이다. key는 첫 cell이며 key column 이름의
  header 아래에 있다. 저장되는 scalar column은 column 순서대로 그 뒤를 잇는다.
- 선언된 title은 column의 header 아래 cell이며 column 순서를 따른다.
- block table에서 배치 foreign key는 결코 cell이 아니며, `parent`는 cell이다.
- cell은 text를 있는 그대로 담는다. 빈 cell은 NULL이고, 빈 텍스트는 `""`로 쓴다. 모든 non-null 저장
  scalar column은 값을 갖는다.
- `"`로 시작하는 cell 값은 `"…"` 안에 `\"`와 `\\`를 escape로 써서 쓴다. `|`는 `\|`로, `|` 앞의 `\`는
  `\\`로 쓴다.

## 한 줄 자리

제목(list 행, H1, cell 안의 제목), list field, field list의 값, table cell은 한 줄 자리다. 그 자리에
줄바꿈을 가진 값, 앞이나 뒤에 공백을 가진 값, 또는 `<` 뒤에 `!`, `/`, 글자가 오는 값은 쓰기에서
`CONSTRAINT`다: 그 자리의 raw HTML은 값을 숨기거나 바꾸기 때문이다. 한 줄 값은 쓴 그대로의 inline
markdown이며, 이 절이 정한 경우를 빼고는 escape하지 않는다.

- text가 공백 하나와 하나 이상의 `#`으로 끝나는 제목은 H1에서 마지막 `#`을 `\#`로 쓴다. CommonMark가
  그것을 닫는 열로 읽기 때문이다.

## `files` 행의 field list

- field list는 H1 줄 바로 뒤에 쓴다: 저장되는 scalar 값마다 non-null인 것 하나씩 `- <column>: <value>`
  항목이며, column 순서이고, 그 뒤에 빈 줄이 온다.
- NULL 값은 없는 항목이다. 빈 텍스트는 콜론 뒤에 아무것도 없는 항목이다: `- <column>:`.
- body는 `-` list 항목으로 시작하지 않는다: body의 첫 줄이 그것인 쓰기는 `CONSTRAINT`다. body는 `*`나
  `+` list나 다른 어떤 블록으로도 시작할 수 있다. 그래서 읽을 때 H1 바로 뒤의 `-` 항목들이 field list다.
- column 순서의 `- <저장되는 scalar column>: <value>` 꼴이 아닌 항목은 `format.block`이다. non-null
  column의 없는 항목은 `format.block`이다. 엔진은 insert에서 default를 명시적으로 쓴다.

## 외래 key의 link

- 외래 key 값은 scalar를 쓰는 모든 자리에서 link `[<key>](<target>)`다: list 행의 field, table의 cell,
  `files` field list의 항목.
- link 텍스트는 참조된 key이며 데이터다. target은 참조된 행에서 유도된다:
  - 참조된 행을 가진 file까지의 상대 경로(모든 플랫폼에서 `/`)에, 그 행이 상세 섹션을 가질 때에만
    `#<anchor>`를 붙인다;
  - 참조된 행 자신의 file 안의 link는 `#<anchor>`만이며, 행이 anchor를 갖지 않으면 그 file의 이름이다.
- 행의 상세 섹션을 더하거나 빼는 쓰기는 같은 transaction에서 그 행을 가리키는 모든 link를 다시 쓴다.
- 유도된 것과 다른 target은 `format.noncanonical`이다. 어떤 행도 가리키지 않는 key는 dangling 참조다.
  참조된 행의 key 갱신은 `FOREIGN_KEY`(restrict-only)이므로 target은 link 아래에서 움직이지 않는다.
- `files` 행에 쓴 `parent`도 link다. block 행의 배치 foreign key는 쓰지 않는다.
- 예: `comments/C5.flowmark.md` `files` 행은 자기 post를 `[P3](../posts/P3.flowmark.md)`로 link한다;
  `posts/P3.flowmark.md` 안의 block 댓글은 자기 작성자를 `[max](../people.flowmark.md)`로 link한다;
  루트 `items.flowmark.md`는 같은 file 안에 상세 섹션을 가진 행을 `[T2](#items-row-t2)`로 link한다.

## Encoding

BOM 없는 UTF-8. canonical 줄 끝은 LF이고, 입력은 CRLF를 유효하다고 받으며(P2) canonical이 아니라고
보고한다(P1). 그 밖의 payload byte는 있는 그대로 보존한다.

## 진단

위의 모든 규칙은 `{file, line, column, rule, message}`를 보고하며 규칙 이름은
`format.noncanonical`, `format.section`, `format.block`, `format.encoding`이다. walk의 규칙
`corpus.walk`는 [document model](20-document-model.md)에 적는다.
