<!-- doc-id: spec-canonical-format -->
<!-- source-sha256: cd41d1a5d7fafa31223de8a4b67f341af1dc608e8a068b47c7dc9bb76bd08472 -->
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
- 구조는 평범한 markdown이다: heading, list, table, field list. 모든 관계는 GitHub, GitLab, VS Code, MkDocs가
  그 heading에 대해 만드는 anchor인 `#<anchor>`를 가진 link다([상세 heading과 anchor](10-canonical-format.md#detail-headings-and-anchors)의 규칙에 따른다).
  Pandoc은 주장 대상이 아니다: 기본 설정은 상대 경로 link target을 다시 쓰지 않으므로, corpus file 사이의 link가
  그 출력에서 따라가진다고 주장하지 않는다.
- 근거: 문서는 database의 유일한 저장이므로 읽을 수 있는 문서로 남아야 한다. 기계만 필요로 하는 것은
  catalog(`dbspec/`)나 `.flowmark/`에 속한다.

## Skeleton과 보존 경계

renderer는 정확히 **skeleton**을 소유한다: `files` 행의 H1 줄과 field list, block·상세·markdown column
섹션의 heading 줄, 행의 list 줄과 table 줄, skeleton 요소 사이의 빈 줄. `markdown` column의 값 — body와
column 섹션 — 은 payload다: renderer는 이를 행과 함께 옮기고 그 byte를 고쳐 쓰지 않는다. 다만 `flowmark canon`이 없애는 줄 끝과 BOM은 예외다([Encoding과 줄 구조](#encoding과-줄-구조)).

- canonical이 아닌 입력(BOM, CRLF, canonical이 아닌 형태의 skeleton 줄, 저장 순서를 벗어난 행)은
  `format.noncanonical` 진단이다. `flowmark canon`은 문서를 canonical 형식으로 고쳐 쓰는 일회성
  migration transaction이고, 논리적 쓰기는 부작용으로 정규화하지 않는다.

## 문서 순서

- **`files` 행**은 `<table>/<key>.flowmark.md` file이며, 차례로: H1 줄 `# <title>`, 저장되는 scalar 값의 field list(있을 때), body, 행 scope의 level-2 섹션을 가진다. 이 요소들 사이의 빈 줄은 field list와 값의 framing에서 정한다.
- **`document` file** `<table>.flowmark.md`는: H1 줄 `# <table>`, H1 바로 아래 행들의 list나 table, level 2의 상세 섹션을 가진다. 없는 document file은 빈 table이다([document model](20-document-model.ko.md#저장)). 마지막 행을 지우면 file은 H1 줄과 LF 하나를 남긴다. list나 table은 행들의 H1 줄 바로 아래에 빈 줄 없이 서며, 그 자리의 빈 줄 하나 이상(space와 tab만 있는 줄을 빈 줄이라 한다)은 `format.noncanonical`이다. 근거: ATX heading은 한 줄이므로 CommonMark 0.31.2와 GFM은 그 뒤의 list나 table을 빈 줄이 있든 없든 같게 읽으며, 모든 viewer가 둘을 같게 보여 준다. 행들은 H1이 이름 붙이는 것이며, block의 행이 그 heading이 이름 붙이는 것인 것과 같이 둘 다 heading 바로 아래에 서고, 빈 줄은 두 요소, 즉 행들과 첫 상세 섹션을 나눈다.
- **상세 섹션**은 heading `<h> <table> row <key>` — table 이름, 공백 하나, `row`, 공백 하나, key — 이며 제목 사본은 없다. 그 뒤에 행의 body가 오고, level h + 1의 행 scope 섹션이 뒤따른다. 상세 행의 scope는 level h + 1이다. 중첩된 list 행의 상세 섹션은 평평한 형제다.
- **블록 섹션**은 부모 행 scope 안에서 자식 table 이름을 가진 level b heading이다. list나 table은 heading 바로 다음 줄에 서며, 그 행들의 상세 섹션이 level b + 1로 뒤따른다. heading과 list나 table 사이의 빈 줄(space와 tab만 있는 줄)은 개수와 무관하게 `format.noncanonical`이다. 근거: ATX heading은 그 줄에서 끝나므로 CommonMark와 GFM은 그 뒤의 list나 table을 빈 줄이 있든 없든 같게 읽으며, 뜻은 확정적이다. list나 table은 heading 고유의 요소이고, document file의 행이 H1 바로 아래에, `files` 행의 field list가 H1 바로 아래에 서듯 heading 바로 아래에 서므로, 형식은 하나뿐이다(P1).
- 한 list나 table의 행들 — document file의 것이든 블록 섹션 하나의 것이든 — 은 하나의 **block**이며, document file은 block 하나를 가진다. block이나 document 행의 `parent`는 그 자신의 block의 행을 가리킨다: 그 밖의 것을 정하는 쓰기는 `CONSTRAINT`이며, 읽을 때 `parent`가 그 block의 행을 가리키지 않으면(file의 다른 block의 행을 포함해서) `format.block`이다. 근거: `parent`는 자기 자신을 가리키는 self foreign key이며, 형제 행은 배치 부모와 `parent`를 공유하므로([document model](20-document-model.ko.md#순서)), 다른 block의 부모는 자식에게 저장 순서의 자리를 주지 못한다. list는 `parent`를 들여쓰기로 쓰며 그것은 다른 block에 닿을 수 없으므로, 더 넓은 scope는 table이 list 형식으로 표현할 수 없는 계층을 담게 한다. 이는 같은 행을 두 형식 중 어느 것으로도 저장할 수 있다는 규칙([document model](20-document-model.ko.md#저장))에 어긋난다.
- 행 scope의 섹션은 고정된 순서를 따른다: body가 아닌 `markdown` column은 column 순서대로, 그다음 자식 블록은 table 이름 순서대로(dbspec의 정준 순서). 상세 섹션은 행의 저장 순서를 따른다. 근거: 순서는 catalog의 함수이므로 행은 하나의 형식을 가진다.
- block이나 document의 list나 table 뒤의 내용은, 그 행들의 상세 섹션을 제외하고 모두 `format.block`이다. 이 내용은 block 섹션에서는 level b 이하의 다음 최상위 heading까지, document 파일에서는 파일 끝까지다.
- null이 아닌 저장 scalar가 없는 `files` 행은 field list가 없으며, 그 첫 body나 섹션은 H1 뒤 빈 줄 정확히 하나 다음에 오고, 빈 body는 아무것도 쓰지 않는다. 근거: field list는 H1 자신의 항목 블록이라 H1 바로 뒤에 서고, body나 섹션은 별개의 요소다. 그 밖의 빈 줄 수는 `format.noncanonical`이다.
- block이나 document 행은 body가 비어 있지 않거나, 그 table에 body가 아닌 non-null `markdown` column이 있거나, body가 아닌 nullable `markdown` column이 NULL이 아니거나, 자식 블록 행을 가질 때 상세 섹션을 가진다. 이 중 어느 것도 없는 상세 섹션은 `format.noncanonical`이다. 상세 섹션 없는 행은 빈 텍스트를 body로 갖는다. 근거: non-null 섹션은 언제나 쓰이므로 그 행은 언제나 상세 섹션을 가지며, 쓸 것이 없는 행은 상세 섹션이 없다.
- 블록의 어떤 행도 가리키지 않는 key의 상세 섹션은 `format.block`이다. 같은 행의 두 번째 상세 섹션도 표기와 무관하게 `format.block`이다: 그 본문과 섹션을 배정할 수 없기 때문이다.
- 상세 섹션은 place로 block에, key로 그 block의 행에 배정된다. block의 두 행이 공유하는 key를 가리키는 상세 섹션은, 그중 하나만 상세 섹션을 가져도 `format.block`이다: 그 섹션을 배정할 수 없기 때문이다. 서로 다른 block의 행들이 file 안에서 바이트 단위로 공유하는 key는 배정을 방해하지 않는다: 그중 하나만 상세 섹션을 가지면 file은 읽히며, 두 행은 `key.duplicate`이고 그 table은 사용할 수 없게 된다([사용할 수 없는 table](#진단)); 둘 다 상세 섹션을 가지면 heading이 충돌하여 file은 `format.block`이다([상세 heading과 anchor](#상세-heading과-anchor)). 근거: block의 상세 섹션은 그 block 섹션 안에 서므로 place가 block을 정하며, block 안의 key만이 행을 정하지 못할 수 있다. 대소문자를 무시하면 같은 key인 행들은 각각 하나의 행과 맞는다. 상세 섹션은 자기가 이름 붙인 key를 바이트 단위로 가진 행을 취하므로, 그런 한 쌍의 행은 배정되며 `key.duplicate`다.
- **저장 순서.** codec은 file의 저장 순서를 [document model](20-document-model.ko.md#순서)이 정하는 대로 행의 집합에서 계산하며, 정렬되지 않은 행의 집합을 거절하지 않는다. file의 행 사이의 `parent` 순환은 읽기에서 `parent.cycle`이며, 그 file은 저장 순서를 갖지 못하므로 읽을 수 없다. 근거: 저장 순서는 행의 함수이므로 행의 집합은 하나의 형식을 가진다.
- layout이 H6를 넘는 heading을 필요로 하면 catalog를 읽을 때 정해지는 schema 오류다.

## 상세 heading과 anchor

- 상세 섹션의 anchor는 `<table>-row-<소문자 key>`다: heading `## items row T2`의 anchor는
  `items-row-t2`다. table은 anchor 텍스트에서 첫 `-` 앞이며, table 이름에는 `-`가 없다.
- **fold.** 두 fold가 heading의 렌더된 inline 텍스트에 대해 충돌을 정한다. 텍스트는 GitHub이 slug를 만드는
  방식을 따른다(link의 target을 뺀 link 텍스트, 강조 표지 제거, code span 내용 유지, entity 해석, raw HTML 태그는
  텍스트가 아님을 포함한다). F_G는 Unicode 일반 범주가 letter(L)나 십진 숫자(Nd)가 아닌 모든 문자를 지우고,
  나머지를 소문자로 바꾸고, NFKD로 분해하고, 결합 기호를 버리며, 남은 ASCII 문자와 숫자만 남긴다. F_M은 텍스트를
  소문자로 바꾸고, NFKD로 분해하고, 결합 기호를 버리며, 남은 ASCII 문자와 숫자만 남긴다. 근거: GitHub, GitLab,
  VS Code의 anchor는 글자, 숫자, `-`, `_`, 공백을 남기고 나머지를 지우며, 그 가운데 위첨자와 원 숫자(범주 No)도
  지운다. F_G가 그것을 지운다. MkDocs는 텍스트의 NFKD 형식에서 ASCII 문자와 숫자만 남기는데 F_M이 그것을 남기고,
  F_G는 그렇지 않으므로 두 fold를 쓴다. `café`는 두 fold 모두에서 `cafe`가 된다.
- **충돌 규칙.** heading은 모든 ATX heading(block-quote나 list-item 표지 뒤의 것을 포함)과, fence 밖에서
  setext 밑줄이 뒤따르는 모든 문단이다. 그 문단 전체가 heading이다. 상세 heading도 heading이다. 상세 heading이 아닌
  heading H는 F = F_G 또는 F = F_M 아래에서, F(H)와 F(A)가 각각의 끝 십진 숫자 run을 뺀 뒤 같으면 상세 heading의 anchor A를
  가져간다. anchor의 fold는 두 fold에서 같다. anchor의 글자와 숫자는 ASCII이기 때문이다. 두 상세 heading은 anchor가 같을 때에만
  충돌한다. 근거: 명명된 viewer들은 반복되거나 점유된 id를 숫자를 덧붙이거나 끝 숫자를 올리는 방식으로만 번호 매긴다(GitHub,
  GitLab, VS Code는 `-<n>`을 덧붙이고, MkDocs는 `_<n>`을 덧붙이거나 끝의 `_<n>`을 올리며, attribute list가 설정한 id도 점유된 것으로 센다).
  그래서 끝 숫자를 뺀 fold를 비교하면 어느 heading이나 id가 먼저 왔든 모든 번호 매기기를 다룬다. 그 비용은 상세 heading과
  끝 숫자만 다른 payload heading도 거절된다는 것이다. 허용되는 예: 한 file의 key `T1`과 `T10`은 둘 다 상세 heading이므로
  결코 충돌하지 않는다. 거절되는 예: `items row T_1` 상세 heading 옆의 payload heading `items row T`는 거절된다. `Notes`는
  그렇지 않다. 이 검사는 file의 모든 heading — 골격과 payload 모두 — 에 적용한다. 또한 file의 payload에 있는 attribute list
  `{…}`(앞의 `:`가 있거나 없는)로서 `#A`로, 또는 `id=` 뒤에 A를 그대로나 작은따옴표·큰따옴표 안에 두어 id를 A로 정하는 것과, 값이 A인 HTML `id` 또는 `name`을
  거절하며, 모두 대소문자를 무시하고 비교한다. 근거: Python-Markdown의 attr_list 확장(MkDocs가 켤 수 있다)은 heading이나
  block 끝의 그런 list를 id로 읽고, HTML viewer는 `id`와 `name`을 anchor로 읽으며, heading의 텍스트와 무관하다.
- 충돌을 새로 만드는 쓰기는 `CONSTRAINT`이며, anchor를 차지하는 heading이나 id를 쓴 행과 column — `markdown` column의 값, `files` 행의 제목(H1에 대해), 또는 section이나 block heading에 대해서는 그것이 여는 행 scope의, 이름 붙인 `markdown` column이나 자식 table — 에 위치하고, 차지한 anchor의 상세 heading을 밝힌다: 제목 갱신, payload 쓰기, 상세 섹션을 만드는 행
  삽입, key 갱신, 이동, 상세 섹션의 생성. 다만 행에 같은 table의 다른 key와 대소문자를 무시하고 같은 key를 주는 쓰기는
  `DUPLICATE_KEY`이며, 그것이 key 규칙이다([document model](20-document-model.md#keys)). 상세 섹션이 없는 행은 anchor가
  없으므로 아무와도 충돌하지 않는다. 읽기에서 충돌은 `format.block`이다. 다만 key가 대소문자만 다른 두 상세 heading 사이는
  `key.duplicate`뿐이다. 같은 file 안에서 같은
  heading 텍스트를 가진 두 상세 섹션은 충돌하므로 `format.block`이며, 어느 행에도 배정할 수 없다.
- 공백이 더 있거나, hash 뒤에 tab이 있거나, table 이름이나 `row`의 대소문자가 정식과 다른 상세 heading이나 section heading은 `format.noncanonical`이며, `flowmark canon`이 고친다. hash 뒤의 canonical 구분자는 공백 하나이며, tab은 쓰이지 않는다. 다만 CommonMark는 `##` 뒤에 tab이 와도 같은 heading으로 읽는다. 근거: table 이름은 상세 level의 자리가, `row`는 문법이 정하며, 이름 붙은 모든 viewer가 anchor에서 둘을 소문자로 만들므로, 정식과 대소문자만 다른 heading은 같은 행을 같은 anchor로 가리킨다. 둘은 viewer가 소문자로 만든 뒤 정식 텍스트와 같을 때 맞는다: U+017F는 `s`가 아니며, U+212A는 소문자로 `k`가 된다.
- 자기 table의 `<table> row <key>`가 아닌, 상세 level의 list나 table의 heading — 대소문자를 무시하고 비교하여 다른 table의 이름을 가진 것(예컨대 `items.flowmark.md`의 `## people row P1`), 또는 다른 어떤 텍스트든 — 은 `format.block`이다. 근거: 상세 level의 heading은 그 앞의 행 scope를 끝내고 이 table의 scope를 열지 않으므로, 그 내용은 file이 가진 어느 행에도 속하지 않으며, `flowmark canon`은 그것이 어디에 속하는지 알 수 없다. key는 대소문자를 구분한다: `## items row t1`은 `format.block`이다.

## 섹션과 그 값

- level L 행 scope의 body와 모든 `markdown` column 값은 다음 최상위 level ≤ L heading에서 끝난다. CommonMark 0.31.2가 해석하는 block 구조가 기준이다: 최상위 heading은 최상위의 ATX heading이거나 최상위 문단을 닫는 setext heading이며, fence, 모든 type(1–7)의 HTML block, 컨테이너(block quote나 list 항목), 게으른 연속 줄, 문단을 끊을 수 있는 것에 관한 규칙은 CommonMark가 정한 그대로 센다. 값은 L보다 깊은 최상위 heading만 담으며, 최상위 level ≤ L heading을 가진 값의 쓰기는 `CONSTRAINT`다. 근거: 파일은 viewer가 읽는 방식으로 읽히므로 값이나 섹션은 CommonMark가 해석하는 것만 될 수 있다. 컨테이너 안의 heading은 그 컨테이너의 내용이지 행의 섹션이 아니며, 더 좁은 열거는 구성 일부(HTML type 6과 7, 게으른 연속 줄)를 놓치거나 해롭지 않은 것(닫힌 컨테이너 안의 열린 block)을 거절한다.
- body가 아닌 모든 `markdown` column은 level L 행 scope의 column 이름을 가진 섹션이다: non-null column의 섹션은 언제나 있고, nullable column의 섹션은 값이 NULL일 때에만 없다. 없는 non-null 섹션은 읽기에서 `format.block`, 쓰기에서 `CONSTRAINT`다. 비어 있는 섹션은 빈 텍스트다. body는 NULL이 아니며 섹션이 아니다. 자식 블록은 자식 table 이름을 가진 level L heading이다. 행 scope 하나에 자식 블록은 최대 한 번 선다: 같은 자식 table 이름을 가진 level L heading의 두 번째는 `format.section`이다. 근거: 행은 자식 table마다 행의 집합을 하나씩 가지며, 두 list나 table은 어떤 규칙도 합치거나 고르지 않는 두 집합을 주므로(git merge의 양쪽이 모두 남거나 오래된 사본이 남는 경우) 그 뜻이 확정적이지 않다. 쓰기는 두 개를 결코 만들지 않는다. 선언되지 않은 level L heading은 `format.section`이다. 근거: non-null 값은 언제나 쓰이므로 그 섹션이 없을 수 없고, NULL 값에는 쓸 텍스트가 없으므로 섹션이 없다.
- 값은 file 안에서 framing된다: 값이 비어 있지 않을 때에만 heading 뒤에 빈 줄이 서고, 값의 줄들이 따르며, 뒤따르는 요소가 있으면 그 앞에 빈 줄이 선다. 값은 줄 바꿈으로 시작하거나 끝나지 않으며, 양 끝에 빈 줄(space와 tab만 있는 줄, CommonMark의 빈 줄)을 담지 않는다. 그런 값의 쓰기는 `CONSTRAINT`다. 근거: framing 빈 줄은 요소를 가르므로, 줄 바꿈으로 시작하거나 끝나는 값은 framing이 바뀐 채 읽히고, 양 끝의 빈 줄은 framing 줄로 읽힌다.
- framing 빈 줄은 skeleton이다: 읽을 때 값의 양 끝에 있는 추가 빈 줄(space와 tab만 있는 줄)은 값의 일부가 아니며 `format.noncanonical`이며, 요소가 여전히 다시 읽히는 framing 빈 줄의 누락도 `format.noncanonical`이다. 다만 field list의 마지막 항목 바로 뒤에 오는 body 줄은 `format.block`이다. `flowmark canon`이 둘 다 고친다.
- 값은 최상위 setext heading을 담지 않는다: 값의 최상위 문단의 줄 바로 뒤에 `=`만 또는 `-`만 있는 줄(들여쓰기 최대 세 칸, 뒤의 공백이나 tab은 있어도 된다)은 쓰기에서 `CONSTRAINT`이며, 읽기에서는 CommonMark가 setext heading으로 읽는 곳이 heading이다. 근거: 최상위 setext 밑줄은 값의 마지막 줄을 최상위 heading으로 바꾸며, setext heading은 level 1 또는 2이고 모든 행 scope의 level L은 2 이상(`files` 행은 2, level h의 상세 섹션 안에서는 h + 1 ≥ 3)이므로 L보다 깊은 setext heading은 없다: 값의 모든 최상위 setext heading이 섹션 경계이며, 이 규칙이 그 모두를 거절한다.
- 값이 fence나 HTML block을 열린 채로 남기는 것은, 그 block이 뒤따르는 요소를 CommonMark 구조에서 흡수할 때에만 거절한다: 쓰기에서는 `CONSTRAINT`이며, 그 요소를 어느 쓰기가 놓든 마찬가지이고 검사는 쓰기가 만드는 file 전체에 대해 돈다. 읽을 때 그런 block이 흡수한 줄은 CommonMark block 구조에서 경계가 아니고 viewer도 그 block 안에 보여 주므로 값의 텍스트이며, 그에 대해 보고할 읽기 결함은 없다. 뒤따르는 최상위 요소가 닫는 컨테이너(block quote나 list 항목) 안에 열린 채 남은 block은 결함이 아니다. 값이 열어 둔 최상위 fence는 그 닫는 fence나 file 끝까지의 모든 줄을 흡수하므로, 읽을 때 값은 그 줄들을 지나 다음 최상위 level ≤ L heading이나 file 끝까지 이어지며, fence 안의 heading은 경계가 아니다(CommonMark가 그렇게 읽는다). 근거: 뒤따르는 요소를 흡수하는 열린 fence나 HTML block은 그 요소를 값의 텍스트로 만들므로, 그 요소를 놓는 쓰기는 놓은 요소를 저장하지 못한다.
- 자식 블록 섹션은 블록이 행을 하나 이상 가질 때만 쓴다 — 빈 블록과 없는 블록은 모두 행이 0개라는 뜻이므로, 정식 형식은 하나뿐이다(P1). 자식 행이 없는 부모 행은 블록 heading을 갖지 않으며, 빈 블록 heading은 `format.noncanonical`이다.
- `markdown` column은 모든 저장에서 언제나 행 scope의 섹션이다 — field나 cell이 아니다.

## 저장되는 scalar column

table의 저장되는 scalar column은 column에서 key, `list`나 `files` 저장의 `title`, `body`, `markdown` column, `list`의 상태
column, 배치 foreign key, `list`의 `parent`, block이나 document 저장의 `order` column — 논리적 column이며 쓰지 않는다([document model](20-document-model.ko.md#순서)) — 을 뺀 것이다.
key, 제목, 상태, 계층은 각자 자리를 가진다. 저장되는 scalar column은 `files` 행의 field list 항목,
list 행의 field, table의 cell이다. null이 아닌 저장 scalar column은 모두 쓴다 — 엔진은 insert에서
default를 명시적으로 쓴다.

## 읽을 수 있는 값 형식

scalar 값은 dbspec literal이 아니라 읽을 수 있는 형식으로 쓴다. type은 언제나 catalog에서 온다.

- text는 있는 그대로 쓴다. 정수, decimal, float는 canonical 수 형식으로, `true`와 `false`는 그대로 쓴다.
- date는 `YYYY-MM-DD`, datetime은 `YYYY-MM-DD HH:MM:SS[.f…]`, time은 `HH:MM:SS[.f…]`다. datetime과 time은 소수 자릿수가 column의 precision과 정확히 같다.
- uuid는 소문자다. bytes는 padding이 있는 base64로 쓴다.
- foreign key는 link다([외래 key의 link](#외래-key의-link) 참조).
- 엔진은 수와 `true`/`false`를 orm의 `dbspec.CanonicalLiteral(t, v)`로, date, time, datetime, uuid를 `CanonicalLiteral(t, "'" + v + "'")`로 읽고, 결과에서 따옴표를 뗀다.
- `now`는 dbspec이 `datetime` 기본값에서 현재 시각을 뜻하는 keyword이며 결코 저장되는 값이 아니다: 엔진이 insert에서 그것이 뜻하는 시각을 쓴다. `now`로 쓰인 시간 값(date, time, datetime)은 그 type이 아니며(읽기에서 `type`, 쓰기에서 `CONSTRAINT`), text 값 `now`는 text다. 근거: `now`는 쓰는 시점을 가리키므로, 저장된 `now`는 읽을 때마다 다른 시각을 가리키게 된다. text는 column의 길이로 검사하며, 한 줄 자리에서는 [한 줄 자리](#한-줄-자리)의 규칙으로도 검사한다.
- 읽을 때 쓰인 형식이 canonical 형식과 다른 값은 `format.noncanonical`이다. column의 type이 아닌 값은 validator의 `type`이며, file을 읽을 수 없게 하지 않는다: 그 값을 평가하는 요청 — 값을 돌려주거나, predicate, join, 정렬, 그룹, 집계에 쓰는 요청 — 은 진단을 밝히며 `CONSTRAINT`로 실패한다. 값을 배정하는 쓰기가 type이 아닌 값을 쓰면 `CONSTRAINT`다. 쓰기가 배정하지 않는 값의 `type` 진단은 행 자신의 것이든 다른 행의 것이든 commit을 실패시키지 않으며, 그 값을 commit의 제약 검사(unique key, `check`, `limit`)가 평가해야 할 때에만 `CONSTRAINT`로 진단을 밝히며 실패시킨다. 그리고 `flowmark canon`을 포함해 file을 다시 쓸 때마다 그 값의 쓴 텍스트를 바이트 그대로 보존한다. key는 이 규칙에서 값이 아니다: 키의 type이 아니거나 정식 텍스트가 아닌 key는 `format.block`이다([document model](20-document-model.ko.md#key)). 근거: 엔진이 type으로 읽지 못하는 값에는 답도 비교도 없으므로 그 값이 필요한 요청이나 검사는 답할 수 없고, 필요하지 않은 것은 영향을 받지 않는다.

- 쓸 때 수, `true`/`false`, date, time, datetime, uuid는 `CanonicalLiteral`이 주는 형식으로 쓴다(`1.5`를 `decimal(4,2)`에 넣으면 `1.50`; float는 왕복하는 가장 적은 자릿수의 고정 표기, 0은 `0`). `CanonicalLiteral`이 거절하는 값 — scale보다 많은 소수 자릿수(0을 포함), precision − scale보다 많은 정수 자릿수, precision보다 많은 소수 자릿수의 time이나 datetime — 은 `CONSTRAINT`다. bytes는 padding이 있는 표준 base64(RFC 4648 §4)로 쓰며, bytes column에서 그 밖의 텍스트를 읽으면 `type`이다. 근거: orm과의 parity.
- text와 `markdown` 값은 자리가 정한 escape를 빼고 바이트 그대로 쓴다(제목의 `\:`와 `\#`, 인용된 list field나 cell, cell의 `\|`). 그렇게 쓸 수 없는 값 — CR, U+0000, 잘못된 UTF-8, 한 줄 규칙 위반, framing 위반 — 은 `CONSTRAINT`다.
- `flowmark canon`([operations](40-ops.ko.md#canon-연산))이 문서 바이트를 정규화하는 유일한 연산이며, 논리적 쓰기는 결코 정규화하지 않는다.

## List 행

- list 행은 `- [<glyph>] <key> <title>[ <field>…]`이고, table에 상태 column이 없으면
  `- <key> <title>[ <field>…]`이다. glyph는 행 상태의 `checkbox` glyph이며 정확히 한 글자다.
- list glyph가 ` `와 `x`가 아니면 흔한 viewer에서 대괄호가 든 글자로 그대로 보인다. 상태 이름은
  catalog의 것이고, `flowmark.dbs`의 glyph는 그 관례다.
- field는 오른쪽부터 parse한다: token `<선언된 column>:<value>`이고, value는 보이지 않는 문자, `"`, `\`가 없는 bare
  token이거나 `\"`와 `\\`를 escape로 갖는 `"…"`다. 보이지 않는 문자를 가진 값은 따옴표로 쓴다(bare token 안의 보이지 않는 문자는 구분자로 읽히거나 아예 읽히지 않는다). 읽기에서, field의 자리 — field, 또는 제목의 마지막 token — 에 있는 token `<선언된 column>:<value>`가 bare value에 보이지 않는 문자를 가지면 `format.block`이다. 쓰기 규칙이 그 자리에서는 그것을 만들지 않기 때문이다. 제목의 다른 자리에서는 그 token은 이스케이프되지 않은 제목 token이다(아래 참고). field에는 sigil이 없다. 저장되는 scalar column만
  field다.
- 없는 field는 NULL, `col:""`은 빈 텍스트다. 모든 non-null column은 쓰며, non-null field가 빠진 행은
  `format.block`이다.
- 제목 token은 space와 tab 사이의 문자 열이다. 제목 token은 그것이 `<선언된 column>:`으로 시작하면 `<선언된 column>:`과 맞으며, 따라서 `owner:x`와 `owner:`는 맞고 `xowner:y`는 맞지 않는다. 맞는 token은 `<column>\:`로 쓰며, `<선언된 column>\:`으로 시작하는 제목 token은 `<column>:`으로 읽고, 그 밖의 `\:`는 해석하지 않는다. escape는 token의 콜론만 바꾸며, 제목 token 사이의 공백은 바이트 그대로 유지한다. 읽기에서, 이스케이프되지 않은 맞는 제목 token이 그 뒤에 다른 제목 token이 오면 `format.noncanonical`이다. 제목의 마지막 token — 첫 field 바로 앞, 또는 field가 없는 행의 마지막 token — 은 field가 서는 자리에 서므로, 그 자리에 있는 이스케이프되지 않은 맞는 token이 올바른 field가 아니면(`owner:`, `owner:a"b`) `format.block`이다: field가 잘못 쓰인 것이거나(빈 텍스트는 `owner:""`다) 제목 텍스트일 수 있기 때문이다. `<선언된 column>\:`으로 시작하는 제목 token은 쓰기에서 `CONSTRAINT`다: 그것은 `<column>:`으로 다시 읽히기 때문이다. 제목은 key와 첫 field 사이의 텍스트다. 근거: field는 오른쪽에서부터 파싱되므로, 마지막 token이 맞는 제목은 field로 다시 읽힐 것이며, 맞는 모든 token을 어디에 있든 이스케이프하면 제목의 쓰인 형식은 하나가 된다. 파싱된 field 뒤에 제목에 남고 그 뒤에 다른 제목 token이 오는 맞는 token은 자리가 확정적이며, viewer는 `\:`를 `:`로 보여 주므로, 이스케이프되지 않은 표기는 정식 표기와 다를 뿐이다. 제목 token은 field를 가르는 것과 같은 space와 tab으로 나뉜다.
- 자식 행은 부모보다 두 칸(CommonMark list 항목 내용 들여쓰기) 더 들여 쓴다. 들여쓰기가 `parent` column이며 다른 것이 이를 쓰지 않는다.
- 배치 foreign key와 `parent`는 block list 행의 field로 쓰지 않으며, 쓰면 `format.block`이다.

## Table 행

- table의 header 행은 저장되는 scalar column을 이름 붙인다. 저장되는 모든 scalar column은 header를
  가지며, header가 없거나 어떤 column도 이름 붙이지 않는 header는 `format.block`이다. key는 첫 cell이며 key column 이름의
  header 아래에 있다. 저장되는 scalar column은 column 순서대로 그 뒤를 잇는다.
- table의 delimiter 행은 column마다 `---`이며 `| --- | --- |`로 쓴다. GFM이 header의 delimiter 행으로 읽는 줄은 `format.noncanonical`이다: 들여쓰기가 최대 세 칸이고, cell들이 `|`로 나뉘며, 각 cell은 하나 이상의 하이픈이고 양 끝에 최대 하나의 콜론이 있으며 그 주위에는 space, tab, 수직 탭(U+000B), 폼 피드(U+000C) 중 어느 것이든 올 수 있고, 줄 양 끝의 `|`는 선택이며 그 뒤에는 이 네 문자 중 어느 것이든 올 수 있고, cell 수는 header와 같다. 다만 앞에 `|`가 없는 줄이 CommonMark가 먼저 다른 block으로 읽는 경우는 예외다: 하이픈만 있고 뒤에 space나 tab만 붙은 줄(setext 밑줄이며 header를 heading으로 만든다), 또는 들여쓰기 뒤 첫 문자가 `-`이고 그 뒤에 space나 tab이 오는 줄(list 항목). 따라서 정렬 콜론은 `format.noncanonical`이다. 그 자리의 다른 줄은 `format.block`이다. 근거: delimiter 행은 skeleton이며 어떤 column도 정렬을 말하지 않으므로, 쓰기가 정렬을 보존할 수 없고 형식은 하나만 정식이다. GFM의 delimiter 행은 cell 주위에 이 네 공백 문자를 허용하므로, 그런 행은 모두 같은 table로 읽히며 `flowmark canon`이 고친다. delimiter 행의 공백은 skeleton이며 한 줄 자리가 아니다. GFM이 delimiter 행으로 읽지 않는 줄은 table을 만들지 않으며, viewer는 header와 행들을 문단 텍스트로 보여 준다.
- 선언된 title은 column의 header 아래 cell이며 column 순서를 따른다.
- block table에서 배치 foreign key는 결코 cell이 아니며, `parent`는 cell이다. 같은 block의 두 행이 공유하는 key를 가리키는 `parent` cell은 `format.block`이다: 부모를 배정할 수 없기 때문이다. 서로 다른 block의 두 행이 공유하는 key는 block이 배정한다.
- cell은 text를 있는 그대로 담는다. 빈 cell은 NULL이고, 빈 텍스트는 `""`로 쓴다. 모든 non-null 저장
  scalar column은 값을 갖는다.
- `"`로 시작하는 cell 값은 `"…"` 안에 `\"`와 `\\`를 escape로 써서 쓴다. table cell에서 값의 각 `|`는 `\|`로 쓰고 다른 `\`는 더하지 않는다. 읽을 때 `\` 바로 뒤에 오는 `|`는 그 `\` 앞에 무엇이 있든 cell 경계가 아니며, 그 `\` 하나를 뺀다. 인용형을 먼저 적용하고 pipe escape를 마지막에 적용하므로, 읽을 때는 pipe escape를 먼저 뺀다. 근거: GFM은 `\` 뒤의 모든 `|`를 cell 텍스트로 읽고 inline 파싱 전에 그 `\`를 뺀다.

## 한 줄 자리

제목(list 행, H1, cell 안의 제목), list field, field list의 값, table cell은 한 줄 자리다. 한 줄 값은 쓴 그대로의 inline markdown이며, 이 절이 정한 경우를 빼고는 escape하지 않는다. 이 절에서 공백(white space)은 Unicode White_Space 속성을 가진 문자다. 보이지 않는 문자(invisible character)는 U+1680을 뺀 공백, Unicode 범주 Cc(제어 문자), Prepended_Concatenation_Mark(U+0600–U+0605, U+06DD, U+070F, U+0890–U+0891, U+08E2, U+110BD, U+110CD)를 뺀 Cf(형식 문자, 예컨대 U+FEFF), Default_Ignorable_Code_Point(예컨대 U+3164, U+115F, U+034F, variation selector)의 문자다. Unicode 속성은 엔진이 빌드되는 Go unicode 패키지의 Unicode 17.0.0 값이다. 이 절의 규칙은 값의 양 끝에 있는 보이지 않는 문자를 거절하며, 값 안의 문자는 끝이 아니다. 구분자 안에서 정식 공백은 space(U+0020)뿐이다. 근거: 그런 문자는 글리프 없이 그려지므로 독자는 볼 수 없다. U+1680과 prepended concatenation mark는 글리프가 있어 텍스트다. 이름 붙은 viewer들은 끝에서 떼어내는 문자 집합이 서로 다르다(Python-Markdown은 White_Space가 아닌 U+001C–U+001F도 떼어낸다). 그러므로 양 끝의 보이지 않는 문자를 모두 거절하면 viewer의 버전과 무관하게 값의 텍스트와 어느 viewer가 보여 주는 것이 같다. CommonMark 0.31.2는 문단과 ATX heading 내용의 양 끝에서 space와 tab을 떼어내며, 이 둘은 이 집합의 일부다.

- 쓰기에서, 그 자리에 줄바꿈이나 CR을 가진 값, 앞이나 뒤에 보이지 않는 문자를 가진 값, 또는 `<` 뒤에 `!`, `/`, 글자가 오는 값은 `CONSTRAINT`다. 줄바꿈은 한 줄에 쓸 수 없고, raw HTML은 값을 숨기거나 바꾸기 때문이다.
- 읽기에서, 렌더링이 inline markdown을 넘어 텍스트와 다르게 보이는 한 줄 값은 `format.block`이며, 정확히 다음이다: 인용된 list field나 인용된 cell(`"…"`)의 따옴표 안 양 끝에 보이지 않는 문자가 있는 것, 또는 `<` 뒤에 `!`, `/`, 글자가 오는 값. 읽기에서, CommonMark가 줄 내용의 끝에서(또는 GFM이 cell의 끝에서) 떼어내는 자리에 있는, 한 줄 자리의 끝의 space나 tab은 `format.noncanonical`이다. list 행의 list 표지 뒤의 space의 연속, 또는 list 행의 key, 제목, field 앞의 space와 tab의 연속은 구분자이며, 그 연속이 space 하나가 아니면, field 앞에서 list 제목을 끝내는 tab을 포함해, `format.noncanonical`이다. 그 이유는 field 파서가 space와 tab을 똑같이 나누고, 쓰기 규칙이 둘 중 하나로 끝나는 제목을 거절하므로, 그 연속은 제목 텍스트가 아니기 때문이다. list 표지 뒤의 tab은 `format.block`이다. 그것이 item 내용의 들여쓰기를 옮기기 때문이다. 제목의 token 사이의 공백은 제목 텍스트이지 구분자가 아니다. 끝에 있는 그 밖의 보이지 않는 문자는 쓰기 규칙이 다시 만들 수 없으므로 `format.block`이다. 각 자리는 하나의 정식 구분자를 가진다(H1 표지 뒤 `# `, field list column 뒤 `: `, 빈 텍스트는 아무것도 없음, list 표지 뒤와, list 행의 glyph, key, 제목, 각 field 사이에 space 하나, cell은 `| v |`). 그 밖의 space나 tab은 `format.noncanonical`이다.
- H1의 제목에서 마지막 `#` 연속이 space나 tab 뒤에 오거나, `#` 문자뿐인 제목은 마지막 `#`을 `\#`로 쓴다. CommonMark가 그 연속을 닫는 열로 읽기 때문이다(`#`만 있는 줄은 빈 heading이다). 읽을 때 제목은 CommonMark의 앞뒤 space, tab, 닫는 열을 뺀 raw heading 내용이며, 마지막 `\#`은 `#`으로 읽는다. 다른 escape는 해석하지 않는다. 쓰기 규칙이 만들지 않을 마지막 `\#`(예: `# C\#`, 제목 `C#`은 `# C#`으로 쓴다)은 `format.noncanonical`이다. 쓰기가 거절하는 해석 텍스트를 가진 제목 — 빈 텍스트(예: `#`, `# #`), 또는 `\#`로 끝나는 텍스트(예: `# a\\#`, `a\#`으로 해석)로, 이것은 `#`으로 다시 읽히므로 `\#`과 `#`, `#\#`과 `##`이 같은 식으로 쓰이게 된다 — 은 읽기에서 `format.block`이고 쓰기에서 `CONSTRAINT`다. 읽기에서 H1의 닫는 열은 `format.noncanonical`이다. 근거: 제목은 그 텍스트 그대로 다시 읽히도록 쓴다. 쓰기가 거절하는 제목은 만들 수 있는 H1이 없으므로 그 file은 왕복할 수 없다(P1, P2).

## `files` 행의 field list

- field list는 H1 줄 바로 뒤에 쓴다: 저장되는 scalar 값마다 non-null인 것 하나씩 `- <column>: <value>` 항목이며, column 순서다. 그 뒤의 빈 줄은 비어 있지 않은 body나 섹션이 뒤따를 때에만, 정확히 그때에만 선다. 둘 다 없으면 행은 마지막 항목에서 끝난다. 근거: 빈 줄은 두 요소를 가르는 구분자이므로, 뒤에 요소가 없으면 가를 것이 없다.
- NULL 값은 없는 항목이다. 빈 텍스트는 콜론 뒤에 아무것도 없는 항목이다: `- <column>:`.
- `files` 행의 body는 `-` list 항목으로 시작하지 않으며, field list 뒤에 두 column 이상(space 또는 tab) 들여쓴 줄로 시작하지도 않는다: 그런 body의 쓰기는 `CONSTRAINT`다. 읽을 때 H1 바로 뒤의 `-` 항목들이 field list이므로, `-` 항목으로 시작하는 body는 `format.block`이며, field list가 앞서 있을 때 첫 줄이 두 column 이상(space 또는 tab) 들여쓴 body도 `format.block`이다. 빈 줄 없이 field list 바로 뒤에 오는 body 줄은 `format.block`이다. CommonMark가 그것을 마지막 항목의 연속으로 읽기 때문이다. body는 `*`나 `+` list나 다른 어떤 블록으로도 시작할 수 있다. 근거: H1 뒤의 `-` 항목은 문법상 field이므로, 그런 body는 쓴 body로 다시 읽힐 수 없다.
- column 순서의 `- <저장되는 scalar column>: <value>` 꼴이 아닌 항목은 `format.block`이다; canonical separator 밖의 space나 tab은 `format.noncanonical`이다([한 줄 자리](#한-줄-자리) 참조). non-null column의 없는 항목은 `format.block`이다. 엔진은 insert에서 default를 명시적으로 쓴다.

## 외래 key의 link

- 외래 key 값은 scalar를 쓰는 모든 자리에서 link `[<key>](<target>)`다: list 행의 field, table의 cell,
  `files` field list의 항목.
- link 텍스트는 참조된 key이며 데이터다. target은 참조된 행에서 유도된다:
  - 참조된 행을 가진 file까지의 상대 경로(모든 플랫폼에서 `/`)에, 그 행이 상세 섹션을 가질 때에만
    `#<anchor>`를 붙인다;
  - 참조된 행 자신의 file 안의 link는 `#<anchor>`만이며, 행이 anchor를 갖지 않으면 그 file의 이름이다.
- 행의 상세 섹션을 더하거나 빼는 쓰기는 같은 transaction에서 그 행을 가리키는 모든 link를 다시 쓴다.
- 유도된 것과 다른 target은 `format.noncanonical`이다. 어떤 행도 가리키지 않는 key는 dangling 참조다: validator가 `reference.dangling`으로 보고하고, 이를 새로 만드는 쓰기는 `FOREIGN_KEY`이며, file을 읽을 수 없게 하지 않는다. 쓴 target은 그대로 간직한다: 유도할 target이 없으므로 target 검사 없이 읽고 읽은 그대로 쓴다. 다만 block이나 document 행의 `parent`가 그 block의 어떤 행도 가리키지 않으면 `format.block`이다. 근거: 없는 행에는 유도할 target이 없고, 쓰기가 바꾸지 않은 target은 그 바이트를 유지해야 하며, 한 block 안의 계층은 저장 순서를 가지려면 완전해야 한다. 참조된 행의 key 갱신은 `FOREIGN_KEY`(restrict-only)이므로 target은 link 아래에서 움직이지 않는다.
- `files` 행에 쓴 `parent`도 link다. block 행의 배치 foreign key는 쓰지 않는다.
- block 행에서 그 행을 담은 file의 `files` 행으로 가는 link는, 자기 file 안에서 anchor가 없는 행에 대한 link와 같이 그 file의 이름이며, codec은 다른 file이나 table 없이 그 file 자신의 key에서 이를 유도한다. 근거: `files` 행은 상세 heading을 갖지 않는다 — 그 H1은 제목이며 제목 갱신이 이를 바꾼다 — 따라서 그 file은 key만으로 유도되는 유일한 target이다. file 이름은 일반 규칙이 주는 상대 경로이며, viewer는 그 행에서 file을 연다.
- 예: `comments/C5.flowmark.md` `files` 행은 자기 post를 `[P3](../posts/P3.flowmark.md)`로 link한다;
  `posts/P3.flowmark.md` 안의 block 댓글은 자기 작성자를 `[max](../people.flowmark.md)`로 link한다;
  루트 `items.flowmark.md`는 같은 file 안에 상세 섹션을 가진 행을 `[T2](#items-row-t2)`로 link한다; `tasks/T4.flowmark.md` 안의 block 행은 T4를 `[T4](T4.flowmark.md)`로 link한다.

## Encoding과 줄 구조

- 문서는 BOM 없는 UTF-8이며, canonical 줄 끝은 LF다. 그 밖의 payload byte는 있는 그대로 보존한다.
- 줄 끝은 CommonMark가 읽는 대로 LF, CRLF, 또는 뒤에 LF가 없는 CR이다. BOM, 그리고 모든 줄 끝의 CR(단독이든 CRLF든)은 P2 아래 유효한 입력이며 `format.noncanonical`이다: canonical 형식에는 둘 다 없으므로 P1이 성립하지 않는다. 읽을 때 값은 모든 줄 끝을 LF로 읽으므로, 단독 CR은 CommonMark에서처럼 줄을 가른다. 근거: multi-platform — `autocrlf`를 쓰는 Windows checkout은 Unix checkout과 같은 값을 읽어야 한다. 이 바이트를 없애는 것은 `flowmark canon`뿐이다.
- UTF-8이 아닌 byte와 U+0000은 `format.encoding`이다: 그것들에서는 어떤 텍스트도 읽을 수 없다.
- 문서는 정확히 LF 하나로 끝난다. 마지막 LF가 없거나 끝에 빈 줄이 있는 문서는 `format.noncanonical`이다. 근거: 문법에는 마지막 줄 뒤의 요소가 없으므로, 그 뒤의 줄 바꿈은 어떤 요소에도 속하지 않아 emit에서 사라진다.
- `format.*` 진단이 있는 file을 다시 쓰는 logical write는 `CONSTRAINT`다, 면제를 제외하고. 다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `order`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([사용할 수 없는 table](#진단)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([사용할 수 없는 table](#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 상세 섹션이 생기거나 사라지는 행으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](40-ops.ko.md#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되는데, file을 지우는 것은 그 file의 바이트를 다시 쓰지 않고 거절이 지키는 것이 바로 그 바이트이기 때문이다. restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 및 이력 block, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다. install이 쓰는 `.gitattributes`가 없으면 Windows `autocrlf` checkout은 모든 값을 읽지만 `flowmark canon` 전까지 위의 면제를 뺀 기존 문서를 다시 쓰는 모든 쓰기를 거절한다. 근거: 쓰기는 사용자가 쓰지 않은 바이트를 바꾸면 안 되며, migration은 별도의 명시적 transaction이다.
- `flowmark install`은 `<corpus>/.gitattributes`에 `*.flowmark.md text eol=lf` 줄을 더하며, 파일이 없으면 만들고 다른 줄은 유지한다. 그래서 git checkout은 모든 플랫폼에서 LF를 유지한다([schema](30-schema.ko.md#install)).

## 진단

위의 모든 읽기 규칙과 walk는 `{file, line, column, rule, message}`를 보고하며, `column`은 규칙이 이름 붙인 요소(glyph, key, 제목, field token, cell 내용, heading 텍스트)의 첫 글자의 1부터 세는 code point 열이다. 줄 전체의 결함은 열 1이다. `format.encoding`이 잘못된 UTF-8 byte에 대한 것이면 그 byte에 있으며, 열은 그 줄에서 그 앞의 code point 수에 1을 더한 것이다: byte는 한 자리를 차지한다. file 전체의 결함이나 텍스트가 없는 corpus 항목(`corpus.walk` 항목, 폴더, symlink 포함)의 결함은 줄 1, 열 1이다. `catalog` 진단도 code point를 센다. 근거: 열은 규칙이 이름 붙인 텍스트의 자리를 정하므로, 글자가 아닌 byte는 디코더가 읽는 자리를 가진다. orm의 dbspec은 lexer에서 code point 단위로 열을 세며(`splitSource`, `lexLine`, `dbspec.go`의 `Diagnostic`), 문서 전체의 결함(크기 한도, byte order mark)은 줄 1, 열 1이다. 읽기 규칙 이름은 `format.noncanonical`, `format.section`, `format.block`, `format.encoding`, `parent.cycle`이고, walk의 규칙은 `corpus.walk`이며, 그 내용은 [document model](20-document-model.ko.md)에 적는다.

- **읽기와 쓰기의 결함.** 읽기에서 다음은 `format.block`이다: 없는 non-null field나 section; 렌더링이 inline markdown을 넘어 텍스트와 다르게 보이는 한 줄 값([한 줄 자리](#한-줄-자리) 참조); `-` list 항목으로 시작하는 `files` body; field list 뒤에서 첫 줄이 두 column 이상(space 또는 tab) 들여쓴 `files` body. 같은 결함은 쓰기에서 `CONSTRAINT`다. 읽기에서, CommonMark가 줄 내용의 끝에서(또는 GFM이 cell의 끝에서) 떼어내는 자리에 있는, 한 줄 자리의 끝의 space나 tab은 `format.noncanonical`이다. list 행의 list 표지 뒤의 space의 연속, 또는 list 행의 key, 제목, field 앞의 space와 tab의 연속은 구분자이며, 그 연속이 space 하나가 아니면, field 앞에서 list 제목을 끝내는 tab을 포함해, `format.noncanonical`이다. 그 이유는 field 파서가 space와 tab을 똑같이 나누고, 쓰기 규칙이 둘 중 하나로 끝나는 제목을 거절하므로, 그 연속은 제목 텍스트가 아니기 때문이다. list 표지 뒤의 tab은 `format.block`이다. 그것이 item 내용의 들여쓰기를 옮기기 때문이다. 제목의 token 사이의 공백은 제목 텍스트이지 구분자가 아니다. 끝에 있는 그 밖의 보이지 않는 문자는 `format.block`이다. 근거: 읽기는 문서가 담은 것을 보고하고, 쓰기는 다시 읽을 수 없는 것을 만들어 내지 않는다.
- **쓰기의 결함.** 거절된 쓰기는 바이트를 쓰지 않으므로, 결함은 file이 아니라 요청에 위치한다: 각각 `{table, key, column, rule, message}`를 보고하며, `table`과 `key`는 규칙이 거절하는 행을, `column`은 규칙이 이름 붙인 값의 column을 가리킨다 — key 결함이면 key column, 제목, field, cell이나 `markdown` column, 자식 table(그 block heading에 대해), `parent`, `order` — 행 전체의 결함이면 비어 있다. 한 쓰기에 준 행들이 key를 바이트 단위로 공유하면, 그런 행의 각 결함은 준 행들 가운데 0부터 세는 index도 가지며, 중복 결함은 첫 행 다음의 그런 행마다 있고 첫 행을 밝힌다. 대소문자를 무시하면 같은 key인 두 행 가운데, 쓰기가 도입하는 행(삽입된 행이나 바뀐 key) 쪽에 중복 결함이 있고 다른 행을 밝힌다. 쓰기가 둘 다 도입하면 byte 순서에서 더 큰 key를 가진 쪽이다. 한 쓰기의 key 검사 — 바이트 단위와 대소문자를 무시한 것 모두, 요청 안과 table의 다른 행에 대해 — 는 어떤 file도 렌더링하기 전에 실행되며, 그것이 거절한 쓰기는 어떤 file도 렌더링하지 않는다. 따라서 이 자리들은 요청의 것이고 그 index는 요청에서 행의 위치다. 형제 순서 결함은 값이 없거나, 정수가 아니거나, 1..m 밖이거나, 형제와 공유되는 그룹의 각 행에, column `order`에 있다. 부모 순환은 순환 위의 각 행에 있으며 column `parent`다. 없는 행을 부모로 가지거나 순환 아래에 있는 행은 자기 결함만 가진다. 다시 써야 하는 file이 읽기 진단을 가지므로 거절된 쓰기는, 그 file을 밝히고 그 진단의 `{file, line, column}`을 싣는다. 근거: 거절된 쓰기가 만들 바이트는 존재하지 않으므로 그 줄들은 열 수 없고, 그 자리는 거절된 텍스트와 묶인 행이 어떻게 놓이는지에 따라 달라질 것이다. 요청은 행을 table과 key로, 값을 column으로 이름 붙이므로, 이 자리는 요청만의 함수다. 이는 SQL 엔진이 위반의 table, 제약, column을 이름 붙이는 것과 같다.
- **validator 규칙.** column의 type이 아닌 값은 `type`이고, dangling 외래 key는 `reference.dangling`이며, 대소문자를 무시하고 같은 table의 다른 key와 같은 key는 `key.duplicate`다. validator는 이것들을 corpus 전체에서 보고한다. 이것들은 file을 읽을 수 없게 하지 않는다. 다만 block이나 document 행의 `parent`가 그 block의 어떤 행도 가리키지 않으면 `format.block`이다([canonical format](#외래-key의-link)), 그리고 같은 block의 두 행이 공유하는 key를 가리키는 `parent` cell도 `format.block`이다([canonical format](#table-행)), 그리고 같은 block의 두 행이 공유하는 key를 가리키는 상세 섹션이나 file 안에서 같은 heading 텍스트를 가진 두 상세 섹션도 `format.block`이다([문서 순서](#문서-순서), [상세 heading과 anchor](#상세-heading과-anchor)). 근거: file의 읽을 수 있음은 다른 file에 의존하면 안 된다. 그렇지 않으면 부모 file을 지울 때 자식이 읽을 수도 고칠 수도 없게 된다. column의 type이 아닌 값의 쓰기는 `CONSTRAINT`이고, dangling 외래 key나 중복 key를 새로 만드는 쓰기는 각각 `FOREIGN_KEY`, `DUPLICATE_KEY`다. 커밋 거절은 [Encoding과 줄 구조](#encoding과-줄-구조)에 적는다. 열기, introspection, `flowmark validate`, `flowmark canon`은 행에 대한 요청이 아니므로 사용할 수 없는 table에서 결코 실패하지 않으며, canon의 링크 유도 거절은 예외다([operations](40-ops.ko.md#canon-연산)). validator는 모든 file의 `format.*`와 `parent.cycle` 진단과, `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 없는 file 안에서 읽을 수 있는 모든 행의 validator 규칙을 보고한다. `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가진 table의 완전한 행을 필요로 하는 규칙 — 그 table로의 `reference.dangling`, `key.duplicate`, 그 안의 `order`와 `state.limit` — 은 그 table에 대해 보고하지 않으며, file 자신의 진단이 그것을 대신한다. 바이트 단위로 같은 key 때문에만 사용할 수 없는 table은 모든 규칙이 보고된다.
- **사용할 수 없는 table.** table은 `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가지거나 바이트 단위로 같은 key를 가진 두 행이 있으면 사용할 수 없다. 이때 요청은 두 번째 행의 file과 `key.duplicate`를 밝힌다. table은 그 storage가 이름을 정한 자리의 모든 corpus 문서를 가진다: `files` table은 모든 `<table>/<name>.flowmark.md`, `document` table은 `<table>.flowmark.md`, block table은 placement parent table이 가진 모든 file을 재귀적으로 가지므로 이력 block은 state table과 함께 사용할 수 없다. table은 `corpus.walk` 항목이 그 자리에, 또는 대소문자를 무시하고 그 자리와 같은 이름에 서 있는 동안에도 사용할 수 없다 — 읽을 수 없는 항목, `files` table의 자리에 있는 symlink나 폴더가 아닌 항목, 또는 다른 대소문자의 suffix, 폴더, document 이름. 근거: 없는 폴더는 빈 table이지만 그 자리의 항목은 폴더도 없음도 아니며, 엔진은 자기가 소유하지 않은 항목의 이름을 바꾸거나 지우지 않으므로 그 table의 행을 읽거나 쓸 수 없다. 대소문자를 무시하고도 `.flowmark.md`로 끝나지 않는 file은 database 밖이며 table을 결코 사용할 수 없게 하지 않는다. 그 행 중 어떤 것이든 필요로 하는 모든 요청 — 키로 하는 point 조회를 포함한 읽기, table의 insert·update·delete, 그 위의 join, relation, subquery, 링크가 그 안에 있을 수 있을 때 행의 상세 섹션이 생기거나 사라지는 행으로의 링크 다시 쓰기, 그리고 그 위의 모든 제약 검사(primary나 unique key, 참조, 링크 target, `limit` 개수, `order` 형제 집합, `check`) — 은 file과 진단을 밝히며 `CONSTRAINT`로 실패한다. 요청은 다른 file들의 행만으로 결코 답하지 않는다. 키의 정식 텍스트가 아닌 이름을 가진 `files` file은 `format.block`이므로 그 table도 사용할 수 없다. 근거: 엔진이 읽지 못하는 행에 대한 답이나 검사는 그 사실을 말하지 않고 틀리게 되며, 요청은 바이트 단위로 같은 key를 가진 두 행 중 하나를 가리킬 수 없다. 엔진은 사용할 수 없는 table에 `flowmark canon`이 file에 하는 것 외에는 아무것도 쓰지 않는다([operations](40-ops.ko.md#canon-연산)): 사람이 corpus를 편집해 그 table을 사용할 수 없게 만드는 행이나 file을 고치고, 이름을 바꾸거나 지우면, 다음 읽기에서 table은 다시 사용할 수 있다. 근거: 요청은 바이트 단위로 같은 key를 가진 두 행 중 하나를 가리킬 수 없고, 엔진이 읽지 못하는 행은 그것을 통해 지울 수 없다.
