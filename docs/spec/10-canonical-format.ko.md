<!-- doc-id: spec-canonical-format -->
<!-- source-sha256: a1cb5dc7e960982cf54de960135cfb3b6dcb95f4eae31a010f73006f791a9bad -->
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
- 구조는 평범한 markdown이다: heading, list, table, field list. 구조 — block, inline, 무엇이 code인지, 무엇이
  heading인지, 무엇이 table인지 — 는 CommonMark 0.31.2로, table과 task-list 항목은 GFM 0.29로 판단한다: GitHub(cmark-gfm),
  GitLab, VS Code(markdown-it)가 구현하는 spec이다. 근거: spec은 이 viewer들이 따르는 고정된 하나의 기준이다. 어느 spec도
  따르지 않는 구현은 list, setext, fence, reference definition, 확장에 대해 자기만의 동작을 가지며 그것은 릴리스와 확장에
  따라 바뀐다. 그것을 따르면 표준이 아니라 한 구현의 일탈을 쫓게 되므로, 그런 구현은 기준이 아니다.
- spec은 fragment의 target을 정의하지 않는다. anchor — heading slug와, CommonMark가 code 밖에서 읽는 raw HTML의 id와
  name — 는 GitHub, GitLab, VS Code로 판단하며(heading에는 그 slug S_GH, S_VS, S_GL), 그 근거는 link `[k](file#anchor)`가 그 viewer들에서
  풀려야 한다는 것이다. 모든 관계는 GitHub, GitLab, VS Code가
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
- **`document` file** `<table>.flowmark.md`는: H1 줄 `# <table>`, H1 바로 아래 행들의 list나 table, level 2의 상세 섹션을 가진다. 텍스트를 소문자로 만들면 table 이름과 같은 H1 — 상세 heading의 table 이름과 같은 방식으로 비교한다([상세 heading과 anchor](#상세-heading과-anchor)), 예컨대 `items.flowmark.md`의 `# Items` — 은 정식이 아닌 표기로 쓴 file의 H1 줄이며 `format.noncanonical`이다. 근거: file 이름이 table을 정하므로 H1은 그것을 이름 붙일 뿐 데이터를 담지 않는다. viewer는 그 아래 행들을 같게 보여 주며, 이름 붙은 모든 viewer가 heading의 anchor를 소문자로 만들므로 그 anchor와 충돌 규칙의 slug는 `# <table>`의 것과 같다. `flowmark canon`은 `# <table>`을 쓴다. 없는 document file은 빈 table이다([document model](20-document-model.ko.md#저장)). 마지막 행을 지우면 file은 H1 줄과 LF 하나를 남긴다. list나 table은 행들의 H1 줄 바로 아래에 빈 줄 없이 서며, 그 자리의 빈 줄 하나 이상(space와 tab만 있는 줄을 빈 줄이라 한다)은 `format.noncanonical`이다. 근거: ATX heading은 한 줄이므로 CommonMark 0.31.2와 GFM은 그 뒤의 list나 table을 빈 줄이 있든 없든 같게 읽으며, 모든 viewer가 둘을 같게 보여 준다. 행들은 H1이 이름 붙이는 것이며, block의 행이 그 heading이 이름 붙이는 것인 것과 같이 둘 다 heading 바로 아래에 서고, 빈 줄은 두 요소, 즉 행들과 첫 상세 섹션을 나눈다.
- **상세 섹션**은 heading `<h> <table> row <key>` — table 이름, 공백 하나, `row`, 공백 하나, key — 이며 제목 사본은 없다. 그 뒤에 행의 body가 오고, level h + 1의 행 scope 섹션이 뒤따른다. 상세 행의 scope는 level h + 1이다. 중첩된 list 행의 상세 섹션은 평평한 형제다.
- **블록 섹션**은 부모 행 scope 안에서 자식 table 이름을 가진 level b heading이다. list나 table은 heading 바로 다음 줄에 서며, 그 행들의 상세 섹션이 level b + 1로 뒤따른다. heading과 list나 table 사이의 빈 줄(space와 tab만 있는 줄)은 개수와 무관하게 `format.noncanonical`이다. 근거: ATX heading은 그 줄에서 끝나므로 CommonMark와 GFM은 그 뒤의 list나 table을 빈 줄이 있든 없든 같게 읽으며, 뜻은 확정적이다. list나 table은 heading 고유의 요소이고, document file의 행이 H1 바로 아래에, `files` 행의 field list가 H1 바로 아래에 서듯 heading 바로 아래에 서므로, 형식은 하나뿐이다(P1).
- 한 list나 table의 행들 — document file의 것이든 블록 섹션 하나의 것이든 — 은 하나의 **block**이며, document file은 block 하나를 가진다. block이나 document 행의 `parent`는 그 자신의 block의 행을 가리킨다: 그 밖의 것을 정하는 쓰기는 `CONSTRAINT`이며, 읽을 때 `parent`가 그 block의 행을 가리키지 않으면(file의 다른 block의 행을 포함해서) `format.block`이다. 근거: `parent`는 자기 자신을 가리키는 self foreign key이며, 형제 행은 배치 부모와 `parent`를 공유하므로([document model](20-document-model.ko.md#순서)), 다른 block의 부모는 자식에게 저장 순서의 자리를 주지 못한다. list는 `parent`를 들여쓰기로 쓰며 그것은 다른 block에 닿을 수 없으므로, 더 넓은 scope는 table이 list 형식으로 표현할 수 없는 계층을 담게 한다. 이는 같은 행을 두 형식 중 어느 것으로도 저장할 수 있다는 규칙([document model](20-document-model.ko.md#저장))에 어긋난다.
- 행 scope의 섹션은 고정된 순서를 따른다: body가 아닌 `markdown` column은 column 순서대로, 그다음 자식 블록은 table 이름 순서대로(dbspec의 정준 순서). 상세 섹션은 행의 저장 순서를 따른다. 근거: 순서는 catalog의 함수이므로 행은 하나의 형식을 가진다.
- block이나 document의 list나 table 뒤의 내용은, 그 행들의 상세 섹션을 제외하고 모두 `format.block`이다. 이 내용은 block 섹션에서는 level b 이하의 다음 최상위 heading까지, document 파일에서는 파일 끝까지다.
- block이나 document의 list나 table과 그 뒤에 오는 요소 — 첫 상세 섹션, 또는 둘러싼 행 scope의 다음 heading — 사이에는 빈 줄 하나가 서며, list나 table이 file을 끝내면 빈 줄은 없다. 읽기에서 빈 줄은 CommonMark 0.31.2가 정의하듯(§2.1) space와 tab만 있는 줄이므로, 그 자리의 그런 줄은 결코 행들 뒤의 내용이 아니다. 그 자리의 다른 수의 빈 줄(없는 것 포함)과 space나 tab을 가진 빈 줄은 `format.noncanonical`이다. 근거: CommonMark는 항목의 내용까지 들여 쓰지 않은 ATX heading에서 list를 끝내고(§5.2), GFM은 빈 줄이나 다른 block의 시작에서 table을 끝내므로(GFM 0.29, Tables), 행들과 그 뒤의 heading은 그 사이 빈 줄의 수와 무관하게 같게 읽힌다. 빈 줄은 값의 framing에서처럼 두 요소를 나누므로 형식은 하나만 정식이다(P1).
- null이 아닌 저장 scalar가 없는 `files` 행은 field list가 없으며, 그 첫 body나 섹션은 H1 뒤 빈 줄 정확히 하나 다음에 오고, 빈 body는 아무것도 쓰지 않는다. 근거: field list는 H1 자신의 항목 블록이라 H1 바로 뒤에 서고, body나 섹션은 별개의 요소다. 그 밖의 빈 줄 수는 `format.noncanonical`이다.
- block이나 document 행은 body가 비어 있지 않거나, 그 table에 body가 아닌 non-null `markdown` column이 있거나, body가 아닌 nullable `markdown` column이 NULL이 아니거나, 자식 블록 행을 가질 때 상세 섹션을 가진다. 이 중 어느 것도 없는 상세 섹션은 `format.noncanonical`이다. 상세 섹션 없는 행은 빈 텍스트를 body로 갖는다. 근거: non-null 섹션은 언제나 쓰이므로 그 행은 언제나 상세 섹션을 가지며, 쓸 것이 없는 행은 상세 섹션이 없다.
- 블록의 어떤 행도 가리키지 않는 key의 상세 섹션은 `format.block`이다. 같은 행의 두 번째 상세 섹션도 표기와 무관하게 `format.block`이다: 그 본문과 섹션을 배정할 수 없기 때문이다.
- 상세 섹션은 place로 block에, key로 그 block의 행에 배정된다. block의 두 행이 공유하는 key를 가리키는 상세 섹션은, 그중 하나만 상세 섹션을 가져도 `format.block`이다: 그 섹션을 배정할 수 없기 때문이다. 서로 다른 block의 행들이 file 안에서 바이트 단위로 공유하는 key는 배정을 방해하지 않는다: 그중 하나만 상세 섹션을 가지면 file은 읽히며, 두 행은 `key.duplicate`이고 그 table은 사용할 수 없게 된다([사용할 수 없는 table](#진단)); 둘 다 상세 섹션을 가지면 heading이 충돌하여 file은 `format.block`이다([상세 heading과 anchor](#상세-heading과-anchor)). 근거: block의 상세 섹션은 그 block 섹션 안에 서므로 place가 block을 정하며, block 안의 key만이 행을 정하지 못할 수 있다. 대소문자를 무시하면 같은 key인 행들은 각각 하나의 행과 맞는다. 상세 섹션은 자기가 이름 붙인 key를 바이트 단위로 가진 행을 취하므로, 그런 한 쌍의 행은 배정되며 `key.duplicate`다.
- **저장 순서.** codec은 file의 저장 순서를 [document model](20-document-model.ko.md#순서)이 정하는 대로 행의 집합에서 계산하며, 정렬되지 않은 행의 집합을 거절하지 않는다. file의 행 사이의 `parent` 순환은 읽기에서 `parent.cycle`이며, 그 file은 저장 순서를 갖지 못하므로 읽을 수 없다. 근거: 저장 순서는 행의 함수이므로 행의 집합은 하나의 형식을 가진다.
- layout이 H6를 넘는 heading을 필요로 하면 catalog를 읽을 때 정해지는 schema 오류다.

## 상세 heading과 anchor

- 상세 섹션의 anchor는 `<table>-row-<소문자 key>`다: heading `## items row T2`의 anchor는
  `items-row-t2`다. table은 anchor 텍스트에서 첫 `-` 앞이며, table 이름에는 `-`가 없다.
- **렌더된 텍스트.** heading의 slug는 그 렌더된 inline 텍스트로 계산한다: CommonMark 0.31.2가 heading의 내용 —
  ATX heading이면 앞뒤의 공백과 tab, 닫는 sequence를 뺀 내용(§4.2), setext heading이면 앞머리의 link reference
  definition을 뺀 문단 — 에서 렌더하는 텍스트로, 그 text node와 code span이 가진 것이다. backslash escape는 escape된
  문자이고, character reference는 그 문자다: numeric reference는 그 code point이며(§2.5) 0은 U+FFFD이므로,
  `&#x83;`은 GitHub과 VS Code가 렌더하는 대로 control U+0083이고, HTML 표준이 attribute 값에서 읽는 windows-1252의
  `ƒ`가 아니다. 강조와 강한 강조의 구분자는 §6.2와 부록의 process emphasis 구분자 알고리즘이 짝지은 곳에서만
  지워지며, 그 밖에서는 텍스트다(`_T1`은 `_`를 남긴다). code span은 §6.1이 정하는 대로 정규화한 내용이다: 줄 바꿈은
  각각 공백이 되고, 양 끝이 모두 공백이며 내용이 공백만이 아니면 양 끝의 공백 하나씩을 지운다. autolink는 그 텍스트이고
  raw HTML은 텍스트가 아니다. link는 target과 title을 뺀 link 텍스트다. 아무것도 이루지 않는 bracket이나 구분자는
  텍스트다. link reference definition은 file 전체의 것이다: definition은 CommonMark가 읽는 곳에서, 즉 문단의 시작에
  있는 block으로, 컨테이너 안이든 밖이든, 그 자리를 가리지 않고 읽는다. label이 그런 definition과 맞는 full,
  collapsed 또는 shortcut reference link는 그 link 텍스트로 센다. 어떤 definition과도 맞지 않는 bracket 텍스트는 label과
  bracket을 포함해 글자 그대로의 텍스트로 센다. 근거: GitHub, GitLab, VS Code는 CommonMark를 해석하며, 거기서 link
  reference definition은 문단의 시작에 있는 block이므로, 정의된 `[items row T1][r]`은 `items row T1` 텍스트로
  렌더된다. 각 viewer는 렌더된 heading의 텍스트, 곧 CommonMark가 렌더하는 것으로 slug를 만들므로, 강조가 아닌 `_`나
  escape된 `\_`는 slug에 남는다. image는 S_GH에서는 텍스트가 아니고, S_VS와 S_GL에서는 그 alt 텍스트, 곧 설명의
  plain-text 렌더다. 근거: GitHub의 id는 image의 alt 텍스트를 넣지 않으며(`a ![b c](/i.png) d`는 `a--d`, 아래의 slug
  vector), VS Code의 preview와 comrak은 heading의 slug를 inline node의 텍스트로 만들며, 거기에는 image의 alt 텍스트가
  포함된다. 줄 바꿈은 soft든 hard든 S_GH와 S_VS에서는 아무것도 아니고 S_GL에서는 공백이다. 근거: GitHub의 id는
  setext heading의 줄을 아무것도 없이 잇고(vector), VS Code는 softbreak와 hardbreak token을 텍스트 없이 쓰며,
  comrak의 `collect_text`는 각각을 공백 하나로 쓴다.
- **math.** math span은 그 viewer가 읽는 대로 읽으며, code span처럼 쪼개지지 않는다: 그 내용은 markup이 아니다.
  GitHub의 renderer는 math를 읽지 않으므로 S_GH는 위와 같이 텍스트를 읽는다. **S_GL**에서 math span은 그 글자
  그대로의 source이며, GitLab이 comrak 0.56.0의 `math_dollars`와 `math_code`로 렌더하는 대로다: backtick이 뒤따르는
  `$`는 code math를 열고, backtick이 앞서는 첫 `$`가 닫으며, 둘 사이의 내용은 비어 있지 않다. 그 밖에서는 `$` 하나나
  둘의 run이 dollar math를 열고, 같은 수의 `$`로 된 다음 run이 닫으며, 내용은 비어 있지 않다 — `$` 하나일 때는 여는
  것 뒤나 닫는 것 앞에 공백, tab, 줄 바꿈이 올 수 없고, backslash 뒤의 `$`는 아무것도 닫지 않으며, 닫는 것 뒤에
  숫자가 올 수 없다. `$` 하나와 code math의 내용은 code span의 것처럼 정규화하고, `$` 둘의 내용은 있는 그대로 쓴다.
  `$` 셋 이상의 run은 아무것도 열지 않으며, 아무것도 열지 않는 `$`는 텍스트다. **S_VS**에서 math span은 텍스트가
  아니며, VS Code의 preview가 `@vscode/markdown-it-katex` 1.1.2로 렌더하는 대로다. 그 plugin은 내장 markdown-math
  extension이 `markdown.math.enabled` 기본값 true로 기여한다: `$$`는 앞에 `$`나 backslash가 없고 뒤에 `$`가 없는 곳에서
  열리고, 짝수 개의 backslash가 앞서며 같은 조건을 갖춘 다음 `$$`에서 닫힌다. `$` 하나는 앞에 `$`, backslash, ASCII
  문자, 숫자, `_`가 없고 guard가 꺼진 곳에서 열리며, 짝수 개의 backslash가 앞서는 다음 `$`에서, 그 뒤에 `$`나 ASCII
  문자, 숫자, `_`가 없을 때 닫힌다. 아무것도 열지 않는 `$`는 텍스트다. guard는 plugin의 식 `/^<\w+.+[^/]>$/`가 맞추는
  inline HTML 구성 요소 뒤에 켜지며, markdown-it이 넣는 다른 모든 token이 그것을 끈다: code span, 어느 문자 앞의
  backslash, character reference, 강조 구분자 run, 둘 이상의 `~` run, 줄 바꿈, autolink, 다른 HTML 구성 요소, math
  span, 그리고 여는 bracket에서부터의 link나 image다. `scheme://` URL 뒤에도 token을 넣는 markdown-it의 linkify는 읽지
  않는다: guard를 켜는 tag, 그런 URL, 그다음 `$`를 담은 heading은 guard가 켜진 채로 읽는다. 근거: comrak의
  `collect_text`는 math node의 literal을 쓰고, VS Code의 `#tokenToPlainText`는 math token을 텍스트 없이 쓰며, slug
  vector가 둘 다 기록한다: `# items row $T<b>1$`은 GitLab에서 `items-row-tb1`, VS Code에서 `items-row`, GitHub에서
  `items-row-t1`이다.
- **S_VS의 math block.** VS Code의 math plugin은 math block도 읽으며, 그것은 다른 viewer가 자기 block으로 읽는 줄들을,
  heading을 포함해, 받아들이므로 S_VS는 VS Code의 block의 heading을 읽는다(충돌 규칙). 네 column 미만으로 들여쓴
  줄로 그 텍스트가 `$$`로 시작하는 것은, 문단을 끊거나 block quote의 lazy 연속을 끝내는 자리에서도, 그 `$$` 뒤의
  텍스트가 `$$`를 두 번 이상 담지 않으면 math block을 연다. 그 텍스트가 `$$`를 한 번, 그 끝에 담으면 block은 그 줄
  하나다. 그 밖에는 block이 뒤따르는 모든 줄을, 빈 줄을 포함해, `$$`를 담은 첫 줄까지 그 줄과 함께, 또는 file의
  끝까지 받아들인다. block quote 안에서는 quote의 lazy 줄 — 빈 줄이 아니었던 quote 줄 뒤의, 문단을 끊는 block을 열지
  않는 줄 — 도 받아들이며, container 안에서는 container가 끝나는 곳에서 끝나고 container가 끝나는 그 줄도 받아들인다:
  block quote는 거기서 닫히고, 내용보다 덜 들여쓴 줄이 끝내는 list 항목은 뒤의 줄들을 위해 열린 채로 남는다. math
  block은 heading도 HTML도 담지 않는다. 근거: `@vscode/markdown-it-katex` 1.1.2의 `math_block` rule로, markdown-it의
  list, reference, HTML block, heading보다 먼저 실행되고 문단, block quote, list를 끊으며, 빈 줄과 무관하게 닫는 줄까지
  훑고, 멈춘 줄의 하나 뒤로 `state.line`을 둔다. slug vector가 각 case를 기록한다(`$$`, `# a`, 빈 줄, `# b`, `$$`,
  `# c`는 VS Code에서 `c`만이고, `- $$`, `  x`, `y`, `  # e`는 항목 안의 `e`다). GitHub과 GitLab은 math block을 읽지
  않는다: 거기서 `$$` 줄은 텍스트이며, vector는 그 heading이 바뀌지 않음을 보인다.
- **S_GH, S_GL, S_VS의 table.** GitHub, GitLab, VS Code는 GFM table(GFM 0.29 §4.10)을 렌더하며, table은 CommonMark가
  문단으로 읽는 줄들을 받아 들이므로 셋은 table 둘레의 heading과 cell의 내용을 각자의 viewer처럼 읽는다(collision
  rule). 각 읽기는 viewer의 source를 따른다: GitHub은 cmark-gfm의 `extensions/table.c`, GitLab은 comrak 0.56.0의
  `src/parser/table.rs`, VS Code는 markdown-it 14.3.2의 `rules_block/table.mjs`.
  - Cell. row의 cell은 backslash 뒤에 오지 않은 모든 pipe에서 나뉘며, 그 backslash가 escape되었든 아니든 같다. code
    span은 어느 pipe도 보호하지 않고, 양 끝의 pipe는 cell을 열거나 닫지 않는다. cell은 둘레의 white space를 뺀
    text이며 — S_GH와 S_GL에서는 공백과 tab, S_VS에서는 JavaScript의 `trim`이 지우는 것 — 각 `\|`를 `|`로 읽는다.
    cell은 inline 내용이다: 그 footnote 참조, raw HTML, id를 문단의 것처럼 읽는다. body row는 header row의 column
    수까지의 cell을 가지며, 그 너머의 cell은 버려진다.
  - GitHub과 GitLab. delimiter row는 들여쓰기 세 칸 이하, 이어서 hyphen에 양 끝의 colon이 선택인 cell들을 pipe로
    구분한 것이며, 그 둘레에 공백, tab, vertical tab, form feed가 올 수 있고 양 끝의 pipe는 선택이다. cell 하나의 row는
    pipe를 가진다. 열린 문단의 한 줄 뒤에 delimiter row가 오면, 그 줄의 cell 수가 같을 때 그 줄이 table의 header
    row이고, 다르면 그 문단은 그 뒤로 table을 열지 않는다. header row 앞의 문단 줄들은 문단으로 남으며 각 `\|`를
    `|`로 읽는다. 그 안의 link reference definition은 text이며 어떤 label도 정의하지 않는다. viewer는 그 문단을 줄들을
    읽은 뒤에 끼워 넣고, 정의는 자기가 가진 문단이 끝날 때만 읽기 때문이다. body row는 빈 줄, 네 column 이상 들여쓴 줄, 문단을 끊는 block이나 어떤 종류든 list item을 여는 줄,
    type 7의 HTML block, pipe 하나뿐인 줄, container의 끝, 그리고 cell 한도를 넘길 row까지 이어진다: table은 body
    row가 지금까지 자동으로 채운 cell — row가 header row에 비해 모자란 cell — 이 GitHub에서 0x80000, GitLab에서
    500000 이하인 동안 row를 받는다.
  - VS Code. markdown-it의 table rule은 모든 block의 시작에서 가장 먼저, 그리고 열린 문단의 줄 가운데 lazy가 아니고
    네 column 이상 들여쓰지 않은 모든 줄에서 돈다. 그래서 ATX heading, fence, HTML block, math block, list item, block
    quote를 열 줄도 뒤에 delimiter row가 오면 table의 header row이고, header row는 그 앞의 문단을 끝낸다. header
    row는 trim하면 pipe를 가지며 네 column 미만으로 들여쓴다. pipe 하나뿐인 줄은 cell이 없으므로 header row가 아니다.
    delimiter row는 열린 모든 container를 lazy가 아니게 이어 가며 네 column 미만으로 들여쓴다. pipe, hyphen,
    colon 가운데 하나로 시작하고, 이어서 그 셋 가운데 하나나 공백이나 tab이 오되 hyphen 뒤의 공백이나 tab은 안 되며,
    그 밖의 것은 가지지 않는다. pipe에서 나눈 모든 부분은 hyphen에 양 끝의 colon이 선택인 것이며, 첫 부분과 마지막
    부분만 비어 있을 수 있다. pipe는 필요 없으므로 `| a |` 아래 `---`는 table이다. header row는 비어 있는 첫 부분과
    마지막 부분을 버리고 delimiter row와 같은 수의 cell을 가진다. list item 문단의 lazy 줄도 같은 방식으로 읽는다:
    그 줄이 header row이면 list item이 끝나고, table은 그 줄이 이어 가는 container 안에서 열린다. block quote의 lazy
    줄은 그렇게 읽지 않는다. body row는 trim하면 빈 줄(no-break space 하나뿐인 줄 포함), 네 column 이상 들여쓴 줄,
    block quote를 끝내는 block(ATX heading, fence, type 1–6의 HTML block, math block, thematic break, block quote, 어떤
    종류든 list item)을 여는 줄, container의 끝, 그리고 cell 한도를 넘길 row까지 이어진다. table은 자동으로 채운 cell을
    0x10000 너머로 늘릴 row를 받지 않으며, header row보다 cell이 많은 row는 음수로 센다.
  - 근거: slug vector가 각 경우를 viewer에서 기록한다. `a|b`, `-|-`, `items row T1`, `---`은 셋 모두에서 heading이
    없다. `# a|b`, `-|-`는 GitHub이 `ab`, VS Code는 없음이다. `| a |`, `---`는 GitHub과 GitLab이 `-a-`, VS Code는
    없음이다. `` `a|b` `` code span 아래 `-|-`는 셋 모두에서 table이다. 그 밖의 경우: `table escaped pipe in a
    cell`, `table escaped pipe in the paragraph before a header`, `table header after a link reference definition
    that a heading uses`, `raw heading in an excess table cell`, `table header
    after a failed header in its paragraph`, `table delimiter row with a vertical tab`, `table rows ended by an HTML
    block of type 7`, `table row of a lone pipe`, `table cell limit`, `table header that a heading reads first`, `table
    in a list item before an unindented line`, `table header on a lazy line of a block quote`, `table header indented
    four spaces in a paragraph`, `table header of a lone pipe`, `table row of a no-break space`.
- **S_GH와 S_GL의 footnote.** GitHub과 GitLab은 cmark-gfm과 comrak 0.56.0의 footnote로 렌더하므로, S_GH와 S_GL은
  block과 참조를 그 viewer처럼 읽는다. VS Code는 footnote를 렌더하지 않으며, S_VS는 bracket과 줄을 CommonMark처럼
  읽는다. 네 column 미만으로 들여쓴 줄로 `[^`, `]`, 공백, tab, 줄 바꿈이 없는 label, `]:`, 그리고 몇 개든 공백과
  tab으로 시작하는 것은 footnote definition을 연다 — 어느 container 안에서든, 문단을 끊는 자리에서도, 그리고 결코 link
  reference definition으로가 아니다. definition은 container다: 그 줄의 나머지가 첫 내용이며, 네 column 이상 들여쓴
  줄 — 그 처음 네 column을 가져간다 — 과 빈 줄에서 이어지고, 그 문단은 다른 block을 열지 않는 줄에서 lazy하게
  이어진다. 그 밖의 줄은 그것을 끝낸다. 그 내용은 선 자리에서 렌더되지 않는다. GitHub에서는 label의 첫 definition이,
  어느 깊이에서든, 그것을 정의한다. 참조가 이름 붙인 label을 정의하는 definition은 문서의 끝에 렌더되고, 그 밖의
  definition은 무엇이 담든 그 내용과 함께 버려진다. GitLab에서는 다른 것이 담지 않은 definition만이 label을 정의하며,
  그 label의 마지막 것이다. label을 정의하는 것은 참조가 이름 붙이면 끝에 렌더되고 아무것도 이름 붙이지 않으면
  버려지며, 아무것도 정의하지 않는 definition은 제자리에 남고, 다른 것 안의 definition은 바깥 것의 내용이다. 버려진
  내용의 heading이나 raw HTML은 anchor도 id도 없다. 렌더된 내용의 heading은 payload heading이다. footnote reference는
  `[` 뒤에 `^`와 한 문자 이상의 label이 오고, link 없이 그것을 닫는 `]`까지인 bracket이다 — inline link나 정의된
  reference link를 먼저 읽는다. GitHub에서 그 label은 `^`와 `]` 사이의 source다. definition이 그 label을 — link label
  처럼 비교해 — 정의하는 참조는 그 definition의 번호를 렌더하며, 번호는 문서 전체 — 그 heading, 문단, table cell,
  image 설명, 그리고 선 자리의 모든 definition의 내용이며, code와 HTML block은 아니다 — 에서 각 definition의 첫 참조의
  순서로 1부터 센다. 따라서 되풀이한 참조는 같은 번호를 렌더한다. 그 밖의 것은 텍스트 `[^`, 쓰인 그대로의 label,
  `]`로 렌더한다. 어느 쪽이든 bracket이 담은 것 — 강조, code, raw HTML — 은 markup이 아니다. GitLab에서는 그 위에 다른
  bracket이 열리지 않았고, 텍스트가 `^`로 시작하며 텍스트와 raw HTML만, `^`보다 많이 담은 bracket이 footnote
  reference이며, 그 구분자는 강조를 열지 않는다: definition이 그 label을 정의하는 것은 텍스트를 렌더하지 않고, 그 밖의
  것은 텍스트로 되돌린다 — `[`, 각 raw HTML 구성 요소를 쓰인 그대로 둔 그 텍스트, `]`이다. 둘 다에서 `![^`는 `!`와
  bracket이며, 결코 image가 아니다. 근거: cmark-gfm과 comrak의 block parser는 footnote definition을 list 항목보다 먼저
  열고, 연속 줄은 네 column 들여썼거나 줄 끝 하나뿐일 때만 받아들인다(`parse_footnote_definition_block_prefix`). 그
  `process_footnotes`는 문서 전체를 걸어 definition에 번호를 매기고, 참조된 것을 그 순서로 끝으로 옮기고 나머지를
  버리며, cmark-gfm은 label의 첫 definition을, comrak은 마지막 바깥 것을 남긴다. GitHub은 참조를 `<sup><a>n</a></sup>`
  로 쓰고 그 heading id는 그 텍스트를 읽으며, comrak의 `collect_text`는 그것을 아무것도 쓰지 않는다. slug vector가 각
  case를 기록한다: `note`와 `q`의 definition 옆의 `# a[^note] b[^Q] c[^x] d`는 GitHub에서 `a1-b2-cx-d`, GitLab에서
  `a-b-cx-d`이다. `items row T1`, `[^n]: x`, `---`는 어느 쪽에서도 heading이 아니다. 참조된 definition 안에 들여쓴
  heading은 문서의 heading들 뒤에 anchor를 받고, 참조되지 않은 것 안의 것은 받지 않는다.
- **S_GL의 wikilink.** GitLab은 pipe 앞에 제목을 두는 comrak의 wikilink로 렌더하므로, S_GL은 열린 bracket이 없는
  곳에서 wikilink를 읽는다: `[[`, 구성 요소 하나, 그다음 `]]`, 또는 `|`, 둘째 구성 요소, `]]`이다. 구성 요소는 `[`,
  `]`, `|`가 아닌 문자의 run이며, backslash는 그 뒤의 ASCII 구두점 문자를 가져가고, 천 byte를 넘지 않는다. wikilink는
  첫 구성 요소 — 그 제목, 또는 pipe가 뒤따르지 않으면 target — 를 공백, tab, 줄 바꿈을 양 끝에서 잘라내고, character
  reference를 decode한 다음 backslash escape를 풀어 렌더하며, 그 markup은 아무것도 렌더하지 않는다: `[[T1|x]]`는 `T1`
  이고 `[[*a* $b$|c]]`는 `*a* $b$`이다. GitHub과 VS Code는 bracket을 CommonMark처럼 읽는다. 근거: GitLab의
  `wikilinks_title_before_pipe`가 설정하는 `WikiLinksMode::TitleFirst`에서의 comrak `handle_wikilink`, 그리고 slug
  vector: `# items row [[T1|x]]`는 GitLab에서 `items-row-t1`, GitHub과 VS Code에서 `items-row-t1x`이다.
- **S_GH의 HTML heading.** GitHub은 자신이 렌더한 HTML에서 다른 heading element가 담지 않은 모든 heading element
  `<h1>`–`<h6>`에, raw HTML의 것에도, 그 element의 텍스트로 anchor를 주며, 그것을 parse된 HTML에서 계산한다: S_GH는
  element를 GitHub의 parser처럼 읽는다. VS Code와 GitLab은 raw HTML heading에 anchor를 주지 않으며, 그 slug는 markdown
  heading만 읽는다. raw HTML heading은 CommonMark 0.31.2가 읽는 raw HTML에 있는 — HTML block 안에, 또는 문단이나
  heading 안에 inline으로, code 안에는 결코 아닌 — 이름의 대소문자를 가리지 않는 start tag `<h1>`–`<h6>`이며, `/>`로
  닫힌 start tag는 빈 element다. HTML은 block마다 읽는다: 최상위 문단은 각각 `<p>`와 `</p>` 사이에, heading은 `<hN>`과
  `</hN>` 사이에 그 렌더된 텍스트와, 선 자리의 raw tag와 함께 렌더된다 — image는 텍스트가 아니며 tag를 담지 않는데,
  GitHub이 그 설명을 alt attribute에 쓰기 때문이다 — 그리고 HTML block은 그 raw 텍스트다. 그 HTML에서 GFM tagfilter의
  tag(`title`, `textarea`, `style`, `xmp`, `iframe`, `noembed`, `noframes`, `script`, `plaintext`)는 GitHub이 escape하는
  대로 텍스트다. comment, processing instruction, declaration, CDATA section은 텍스트가 아니다. HTML block 안의 named
  reference는 그 이름이 HTML 4.01의 것이거나 `apos`이고 `;`로 끝날 때만 decode되며, numeric reference — `&#`와 십진
  숫자, 또는 `&#x`와 십육진 숫자를 뒤따르는 만큼, 그리고 선택적인 `;` — 는 그 code point이고, 숫자가 없거나 Unicode
  scalar value가 아니면 아무것도 아니다. `isindex` start tag는 텍스트 `This is a searchable index. Enter search
  keywords: `를 쓴다. heading element의 텍스트는 그 start tag에서 끝까지의 텍스트다: 자기 end tag가 끝내되, 안에
  `div`, `td`, `th`, `tr`, `thead`, `tbody`, `tfoot`, `table`, `head`, `body`, `html`이 열려 있으면 끝내지 않는다.
  `</body>`와 `</html>`은 끝낸다. start tag `fieldset`, `form`, `li`, `p`, `table`, `h1`–`h6`은 안에 열린 element를
  가장 안쪽부터, vector `nested`가 그 element가 열린 채로 남았을 때 그것이 닫는다고 기록하는 것마다 닫으며, 그것을
  모두 닫으면 heading을 끝내므로, 열린 것이 없으면 끝낸다. 다른 heading의 end tag는 아무것도 아니다. 안에 열린
  element의 end tag는 그 element를 닫는다. 바깥에 열린 element의 end tag는, 안의 element가 그 tag보다 우선순위가 높지
  않으면 heading을 끝낸다(libxml2의 end 우선순위: `div` 150, `td`와 `th` 160, `tr` 170, `thead`, `tbody`, `tfoot` 180,
  `table` 190, `head`와 `body` 200, `html` 220, 그 밖은 100). 자기 tag가 끝내지 않는 다른 heading element 안에서, 또는
  `select` 안에서 시작하는 heading element는 anchor가 없다. 어떤 tag가 element를 끝내는지 engine이 알 수 없는 곳 —
  element의 시작을 담지 않는 강조나 link 안에서, 또는 `nested`가 기록하지 않은 element에서 오는 그 목록의 start tag,
  engine이 적용하지 않는 libxml2 `htmlStartClose`(version 2.9.14와 2.13.5를 합친 것)의 쌍으로 어떤 start tag가 안이나
  바깥에 열린 element를 닫았을 수 있거나, engine이 읽지 않는 내용이 앞에 왔을 때의 모든 tag, heading이 container
  안에 있을 때의 end tag — 에서는 element가 거기서 끝날 수도 이어질 수도 있으며, 두 읽기를 모두 취한다. 자기 block이 끝내지 않는
  element는 다음 최상위 block으로 이어진다: 그 HTML을 읽는 문단, heading, HTML block, 빈 element인 thematic break,
  또는 element를 끝내는 file의 끝이다. **prefix 규칙:** 그 밖의 내용 — container 안의 block, list, block quote, code
  block, 또는 container 안 block의 끝에 오는 어떤 것 — 으로 이어지는 element는 engine이 읽는 범위를 넘어 이어지며,
  지금까지의 slug가 A로 시작하면 anchor A를 가져간다. 이것이 안전한 방향이다: 뒤따르는 것은 element의 텍스트에 덧붙일
  수만 있고 S_GH는 각 문자를 따로 사상하므로, 최종 slug는 지금까지의 slug로 시작하며, A로 시작하는 slug만이 A와
  같거나 A로 번호 매겨질 수 있다. 근거: slug vector가 GitHub의 읽기를 기록한다 — raw heading 안의 start tag와 end
  tag로서의 모든 element 이름, 그리고 열린 채로 남은 각 element 뒤의 것(`closers`, `nested`), entities package의 모든
  named reference(`htmlEntities`), raw `<h2>` 안의 모든 code point(`github-html-codepoints.txt`), raw heading
  case(`rawHeadings`) — 이며, 읽기는 이를 그대로 재현한다. prefix 규칙의 case는 예외로, GitHub의 id가 지금까지의
  slug로 시작한다. 허용되는 예: raw `<h2>Notes</h2>`, 그리고 상세 heading `items row T1` 옆의 텍스트가 `items row T1x`인
  `<h2>items row T1<b>x</b></h2>`. 거절되는 예: 그 옆의 raw `<h2>items row T1</h2>`, 그리고 list가 뒤따르는 닫히지
  않은 `<h2>items row`.
- **slug.** 한 viewer에서 heading의 slug는 그 렌더된 inline 텍스트로부터 그 viewer가 계산하는 대로 계산한다.
  **S_GH**(GitHub): 텍스트를 Unicode의 full default lowercase mapping으로, 조건부 mapping 없이 소문자로 바꾸고(U+0130은
  `i`와 U+0307이, U+212A는 `k`가, 끝의 Σ는 σ가 된다), Unicode 17.0.0에서 Unicode Technical Standard #18(Annex C)의
  word 문자 — Alphabetic, mark, decimal number, connector punctuation, Join_Control이므로 U+200D는 남는다 — 와 공백,
  `-`를 남기고 나머지를 지우며, 그다음 각 공백을 `-`로 쓴다. **S_VS**(VS Code): 텍스트의 양 끝에서 JavaScript의
  `String.prototype.trim`이 지우는 white space를 빼고, JavaScript의 `toLowerCase`(Final_Sigma 조건을 갖춘 full
  mapping이므로 끝의 Σ는 ς)로 소문자로 바꾸며, VS Code의 식 `githubSlugReplaceRegex`가 맞추는 모든 문자 — Unicode
  13.0.0의 일반 범주가 No, Pd, Ps, Pe, Pi, Pf, Po, Sm, Sc, Sk, So, Cc, Cf, Co, Cn, Zs, Zl, Zp인 문자 가운데 Unicode
  13.0.0의 Alphabetic 문자, 공백, `-`가 아닌 것이므로, Unicode 13.0.0 이후에 배정된 문자는 지워진다 — 를 지우고,
  그다음 각 JavaScript white space 문자를 `-`로 쓴다. 남는 것은 공백뿐이다. **S_GL**(GitLab): Rust의
  `to_lowercase`, 곧 S_VS처럼 Final_Sigma 조건을 갖춘 full mapping으로 소문자로 바꾸므로 끝의 Σ는 ς이며, Unicode 17.0.0의 일반 범주가 letter, mark, number, connector punctuation인 문자와 공백, `-`를 남기고 나머지를
  지우며, 그다음 각 공백을 `-`로 쓴다. 어느 것도 텍스트를 정규화하거나, hyphen을 잘라내거나, run을 줄이지 않는다:
  `café`와 `cafe`는 slug가 다르며, `items row  T1`과 `items row T1`도 그렇다. engine의 Unicode data는 Go `unicode`
  package의 것인 17.0.0이며, S_VS가 지우는 집합에는 Unicode 13.0.0 data file로 생성한 table을 쓴다. 근거: slug
  vector, 곧 viewer 자신이 렌더하는 id로, 2026-10-11에 `make slug-vectors`(`scripts/record-slugs.mjs`)가
  `store/testdata/slug`에 기록했으며 S_GH, S_VS, S_GL이 그대로 재현한다: surrogate를 뺀 모든 code point마다 그것을
  numeric reference로, 그리고 그 문자 자체로 담은 heading, 그리고 공백, tab, underscore, hyphen, reference, escape,
  강조, code span, link, image, raw HTML, 줄 바꿈, math span과 block, footnote definition과 reference, wikilink, 반복된
  heading의 구조 case다. GitHub의 것은 자신의 renderer, 곧 REST API의
  `POST /markdown`을 mode `markdown`으로 불러 렌더하며, 이 mode는 각 heading anchor를 `user-content-`와 id로 쓴다. 그
  id는 Unicode 13.0.0 이후에 배정된 U+107A5, U+1CCF0, U+A7F2를 남기며, 모든 code point에서 Unicode 17.0.0의 word
  문자와 맞는다. VS Code의 것은 최신 stable release 1.141.0의 자기 code를 실행해 렌더한다:
  `extensions/markdown-language-features/src/slugify.ts`의 `githubSlugifier` — VS Code의 MIT license와 함께
  `store/testdata/slug`에 두며, 그 식은 engine의 Unicode 13.0.0 table과 모든 code point에서 같다 — 와
  `markdownEngine.ts`의 `#tokenToPlainText` method를, 그 `package-lock.json`이 고정하는 markdown-it 14.3.2 위에서,
  markdown-math extension의 `package-lock.json`이 고정하고 그 `extension.ts`가 설정하는 `@vscode/markdown-it-katex`
  1.1.2와 함께 실행한다. GitLab의 것은 GitLab의 `Gemfile.lock`이 고정하는 version과 sha256의 gitlab-glfm-markdown
  0.0.45를 그 gem 자신의 source에서 build해 — Ruby extension처럼 comrak 0.56.0으로 렌더하는 그 command line binary를,
  Ruby binding 없이 — GitLab의 `GlfmMarkdown` engine의 option(그 가운데 `math_dollars`, `math_code`, `footnotes`)과
  prefix `user-content-`로 실행해 렌더한다. id는 comrak의 `Anchorizer`가 쓰며, 그 gem의 `Cargo.lock`은 comrak이 범주를
  읽는 crate인 finl_unicode 1.4.0을 고정하고, 그 table은 Unicode 17.0.0의 것이다. 다시 기록하려면 network, 인증된
  `gh`, npm, cargo를 갖추고 `make slug-vectors`를 실행한다. viewer가 바뀌면 slug가 더는 재현하지 못하는 vector로
  드러난다.
- **GitLab의 snippet prefix.** GitLab은 snippet에서만 heading의 id 앞에 그 file의 이름을 붙인다: 그 markup blob
  viewer는 blob의 container가 snippet일 때만 `use_filename_in_anchor`를 설정하며, 그때 그 `GlfmMarkdown` engine이 file
  base name의 slug와 `-`를 heading slug prefix로 넘긴다. corpus file은 repository의 file이며, GitLab은 그것을 prefix
  없이 렌더하고, 상세 anchor는 그 렌더링의 anchor이므로 규칙은 prefix를 읽지 않는다. snippet에서 prefix는 모든
  heading의 id 앞에 똑같이 서며, 번호는 prefix 없는 slug로 매기므로, 거기서 heading들은 repository에서와 똑같이 서로의
  id를 가져간다. 근거: GitLab의 `app/models/blob_viewer/markup.rb`, `lib/banzai/filter/markdown_engines/glfm_markdown.rb`,
  `lib/gitlab/heading_slug.rb` — recorder가 읽고 그것이 바뀌면 실패한다 — 그리고 `items.flowmark.md`의 prefix
  `itemsflowmark-`를 붙인 snippet의 id를 기록하는 slug vector(`gitlab-headings.json`의 `snippet`)다.
- **충돌 규칙.** heading은 모든 ATX heading(block-quote나 list-item 표지 뒤의 것을 포함)과, fence 밖에서
  setext 밑줄이 뒤따르는 모든 문단이다. 그 문단 전체에서 앞머리의 link reference definition을 뺀 것이 heading이다. 상세 heading도 heading이다. GitHub이 anchor를 주는 raw HTML heading도 S_GH에서만 heading이며, 그 start tag의 `<`에 위치한다(S_GH의 HTML heading). 각 slug는 그 viewer의 block의 heading을 읽는다: CommonMark의 것에, S_GH와 S_GL에서는 GitHub과 GitLab의 footnote definition을(S_GH와 S_GL의 footnote), S_VS에서는 VS Code의 math block을(S_VS의 math block), 셋 모두에서 table을(S_GH, S_GL, S_VS의 table) 더하고, 그 각각이 렌더하는 raw HTML을 더한 것이다. 상세 heading은 CommonMark의 block의 것이며, 같은 줄, level, 텍스트의 viewer heading이 그 상세 heading이다. viewer가 anchor를 주는 그 밖의 heading은 그 slug에서 payload heading이다. 근거: 행으로의 link가 따르는 anchor는 각 viewer가 자기가 읽는 heading에서 렌더하는 것이므로, 한 viewer만 읽는 heading은 거기서 anchor를 가져갈 수 있고, viewer가 읽지 않는 heading은 거기서 아무것도 가져가지 않는다. 그 definition은 heading의 일부가 아니다: setext heading은 definition이 남긴 첫 줄에서 시작하고, definition은 heading 앞의 내용이며, definition만 있고 밑줄이 뒤따르면 heading이 아니다. 근거: CommonMark 0.31.2는 문단을 setext heading으로 닫기 전에 그 앞머리의 definition을 먼저 읽으므로(§4.3, §4.7), `[r]: /u`, `people row P1`, `---`는 definition 하나와 heading `people row P1`이며, commonmark.js, cmark-gfm, markdown-it이 그렇게 읽는다. 이 규칙에서 상세 heading은 file의 layout에 있는 한 table의 상세 level — 그 행들의 상세 heading의 level, 행 scope보다 한 단계 얕은 — 에 있는 최상위 heading이며, 그 텍스트가 공백, tab, 그리고 setext heading의 줄 바꿈을 뺀 것으로 그 table의 이름과 `row`(각각 viewer가 하듯 소문자로 만든 뒤 정식 텍스트와 같은 것)와 key이다. 즉 상세 섹션의 heading이며, 정식 또는 비정식 철자로 쓰인다. 그 밖의 모든 heading은 payload heading이며, 다른 level이나 block quote, list 항목 안에 있는 `<table> row <key>` heading을 포함해, 그 slug가 모든 상세 anchor와 비교된다. 근거: 상세 섹션은 자리로 배정되므로, 다른 곳의 heading은 어느 행도 가리키지 않으며 자기 anchor를 가지는데, viewer는 그것이 상세 anchor와 반복될 때 번호를 매긴다. 상세 heading이 아닌
  heading H는, S_GH, S_VS, S_GL 가운데 한 slug S와 그 slug가 읽는 대로 렌더된 H의 텍스트 T에 대해 S(T)가 A와 같거나,
  A가 S(T) 뒤에 `-`와 앞자리 0 없이 쓴 십진수 n ≥ 1을 붙인 것이면(번호 규칙) 상세 heading의 anchor A를 가져간다. 두
  상세 heading은 anchor가 같을 때에만 충돌한다. 근거: 명명된 모든 viewer는 반복된 slug에 1부터 세는 `-<n>`을 덧붙여
  번호 매기므로, slug가 S(T)인 heading은 어느 heading이 먼저 왔든 A로 번호 매겨질 수 있으며, 다른 번호는 결코 붙지
  않는다. GitHub은 앞선 id가 가진 번호를 건너뛰고(`a-1`, `a`, `a`는 `a-1`, `a`, `a-2`), slug가 앞선 id인 heading에
  번호를 매긴다(`a`, `a`, `a-1`은 `a`, `a-1`, `a-1-1`). VS Code의 slug builder는 각 slug를 따로 세므로 id를 되풀이할
  수 있다(`a-1`, `a`, `a`는 `a-1`, `a`, `a-1`)(slug vector). comrak의 `Anchorizer`도 `-<n>`을 덧붙인다. GitHub은
  slug가 빈 heading 일부에 anchor를 렌더하지 않는다. 상세 anchor는 비어 있거나 `-`로 시작하지 않는다. 허용되는 예: 한 file의 key `T1`과 `T10`은 둘 다 상세 heading이므로
  결코 충돌하지 않는다. 상세 heading `items row T` 옆의 payload heading `items row T2`, 그리고 상세 heading
  `items row T-01`이나 `items row T-0` 옆의 `items row T`도 허용된다. 어느 viewer도 그런 번호를 쓰지 않기 때문이다.
  거절되는 예: `items row T-1` 상세 heading 옆의 payload heading `items row T`는 거절된다. 두 번째 `items row T`가 `items-row-t-1`로 번호 매겨지기 때문이다. `Notes`는
  그렇지 않다. 이 검사는 file의 모든 heading — 골격과 payload 모두 — 에 적용한다. file 어디서든, 골격까지 포함해, CommonMark 0.31.2가 읽는 raw HTML에서(code span이나 code block에는 없다) A를 가져가는 HTML `id` 또는 `name`을 거절한다: id나 name은 full default mapping으로 소문자로 바꾼 뒤 A와 같거나 `user-content-` 뒤에 A를 붙인 것과 같으면 같다. VS Code가 fragment를 id와 비교하는 방식이다. `user-content-` 뒤에 A를 붙인 것은 GitHub이 heading의 id를 렌더하는 형태이자 GitLab의 id 형태이며, GitLab과 GitHub은 정확히 비교하므로 소문자로 바꾼 비교가 그것들을 덮는다. 근거: HTML viewer는 `id`와 `name`을 heading의 텍스트와 무관하게 anchor로 읽는다. GitHub은 raw `id`나 `name`을 대소문자까지 그대로 남기고, 값이 정확히 `user-content-`로 시작하지 않으면 그 앞에 `user-content-`를 쓴다(`ITEMS`는 `user-content-ITEMS`, `user-content-x`는 그대로, `User-Content-x`는 `user-content-User-Content-x`). 이는 slug vector와 함께 관측했으며, 따라서 A나 `user-content-` 뒤에 A를 붙인 raw `id`나 `name`은 heading의 id로 렌더된다. VS Code는 그것들을 쓰인 그대로 둔다. 어느 viewer도 id의 대소문자를 소문자로 바꾸는 것 너머로 접지 않으므로 `items-row-ſ1`은 `items-row-s1`이 아니다. `{#…}`는 CommonMark와 GFM에서 글자 그대로의 텍스트이며, 어느 기준 viewer도 그것에서 id를 읽지 않는다.
- 충돌을 새로 만드는 쓰기는 `CONSTRAINT`이며, anchor를 차지하는 heading이나 id를 쓴 행과 column — `markdown` column의 값, `files` 행의 제목(H1에 대해), 또는 section이나 block heading에 대해서는 그것이 여는 행 scope의, 이름 붙인 `markdown` column이나 자식 table — 에 위치하고, 차지한 anchor의 상세 heading을 밝힌다: 제목 갱신, payload 쓰기, 상세 섹션을 만드는 행
  삽입, 이동, 상세 섹션의 생성. 다만 행에 같은 table의 다른 key와 대소문자를 무시하고 같은 key를 주는 쓰기는
  `DUPLICATE_KEY`이며, 그것이 key 규칙이다([document model](20-document-model.md#keys)). 상세 섹션이 없는 행은 anchor가
  없으므로 아무와도 충돌하지 않는다. 읽기에서 충돌은 `format.block`이다. 다만 key가 대소문자만 다른 두 상세 heading 사이는
  `key.duplicate`뿐이다. 같은 file 안에서 같은
  heading 텍스트를 가진 두 상세 섹션은 충돌하므로 `format.block`이며, 어느 행에도 배정할 수 없다.
- 읽기에서 충돌은 충돌하는 요소의 자리에 위치한다([진단](#진단)): heading은 그 텍스트의 첫 줄과 그 텍스트의 첫 글자에, HTML `id`나 `name`은 그 tag의 `<`에 위치한다. 이것은 읽기 위치일 뿐이다: 충돌을 새로 만드는 쓰기는 [진단](#진단)의 쓰기 결함과 앞 절이 정하는 대로, anchor를 차지하는 heading이나 id를 쓴 행과 column에 위치하며, 그 메시지는 상세 heading을 밝힌다. 근거: column은 규칙이 이름 붙인 글자를 가리키며, block의 첫 줄은 HTML block 깊숙한 `id`에서 멀리 떨어질 수 있다.
- 정식 공백 하나가 아닌 공백이나 tab의 run(글 앞, 낱말 사이, 또는 글 뒤에 있는 것), hash 뒤에 tab이 있거나, hash 대신 setext 밑줄이 있거나, table 이름이나 `row`의 대소문자가 정식과 다른 상세 heading, section heading, block heading은 `format.noncanonical`이며, `flowmark canon`이 고친다. 그 진단은 그 텍스트의 첫 글자에 위치한다([진단](#진단)). hash 뒤의 canonical 구분자는 공백 하나이며, tab은 쓰이지 않는다. 다만 CommonMark는 `##` 뒤에 tab이 와도 같은 heading으로 읽는다. setext 밑줄도 쓰이지 않는다. 다만 CommonMark는 `-` 밑줄 위의 문단을 같은 level 2 heading으로, `=` 밑줄 위의 문단을 같은 level 1 heading으로 읽으므로(§4.3), setext section heading이나 block heading은 같은 section이나 block을 가리킨다. 근거: table 이름은 상세 level의 자리가, `row`는 문법이 정하며, 이름 붙은 모든 viewer가 anchor에서 둘을 소문자로 만들므로, 정식과 대소문자만 다른 heading은 같은 행을 같은 anchor로 가리킨다. 둘은 viewer가 소문자로 만든 뒤 정식 텍스트와 같을 때 맞는다: U+017F는 `s`가 아니며, U+212A는 소문자로 `k`가 된다.
- 자기 table의 `<table> row <key>`가 아닌, 상세 level의 list나 table의 heading — 소문자로 만든 뒤 다른 table의 이름을 가진 것(예컨대 `items.flowmark.md`의 `## people row P1`), 또는 다른 어떤 텍스트든 — 은 `format.block`이다. 근거: 상세 level의 heading은 그 앞의 행 scope를 끝내고 이 table의 scope를 열지 않으므로, 그 내용은 file이 가진 어느 행에도 속하지 않으며, `flowmark canon`은 그것이 어디에 속하는지 알 수 없다. key는 대소문자를 구분한다: `## items row t1`은 `format.block`이다.

## 섹션과 그 값

- level L 행 scope의 body와 모든 `markdown` column 값은 다음 최상위 level ≤ L heading에서 끝난다. CommonMark 0.31.2가 해석하는 block 구조가 기준이다: 최상위 heading은 최상위의 ATX heading이거나 최상위 문단을 닫는 setext heading이며, fence, 모든 type(1–7)의 HTML block, 컨테이너(block quote나 list 항목), 게으른 연속 줄, 문단을 끊을 수 있는 것에 관한 규칙은 CommonMark가 정한 그대로 센다. 값은 L보다 깊은 최상위 heading만 담으며, 최상위 level ≤ L heading을 가진 값의 쓰기는 `CONSTRAINT`다. 근거: 파일은 viewer가 읽는 방식으로 읽히므로 값이나 섹션은 CommonMark가 해석하는 것만 될 수 있다. 컨테이너 안의 heading은 그 컨테이너의 내용이지 행의 섹션이 아니며, 더 좁은 열거는 구성 일부(HTML type 6과 7, 게으른 연속 줄)를 놓치거나 해롭지 않은 것(닫힌 컨테이너 안의 열린 block)을 거절한다.
- body가 아닌 모든 `markdown` column은 level L 행 scope의 column 이름을 가진 섹션이다: non-null column의 섹션은 언제나 있고, nullable column의 섹션은 값이 NULL일 때에만 없다. 한 column의 섹션은 행 scope 하나에 최대 한 번 선다: 같은 column 이름을 가진 level L heading의 두 번째는, 그 heading을 어떻게 적었든, 그 값이 첫 번째와 같더라도 `format.section`이다. 근거: column은 값 하나를 가지며, 두 섹션은 어떤 규칙도 합치거나 고르지 않는 두 값을 주므로(git merge의 양쪽이 모두 남거나 오래된 사본이 남는 경우) 그 뜻이 확정적이지 않다. `flowmark canon`은 한 값의 바이트를 버려야만 이를 고칠 수 있는데 renderer는 payload에 결코 그렇게 하지 않으며([Skeleton과 보존 경계](#skeleton과-보존-경계)), 두 값을 비교하면 file을 읽을 수 있는지가 그 payload에 달리게 된다. 쓰기는 두 개를 결코 만들지 않는다. 없는 non-null 섹션은 읽기에서 `format.block`, 쓰기에서 `CONSTRAINT`다. 비어 있는 섹션은 빈 텍스트다. body는 NULL이 아니며 섹션이 아니다. 자식 블록은 자식 table 이름을 가진 level L heading이다. 행 scope 하나에 자식 블록은 최대 한 번 선다: 같은 자식 table 이름을 가진 level L heading의 두 번째는 `format.section`이다. 근거: 행은 자식 table마다 행의 집합을 하나씩 가지며, 두 list나 table은 어떤 규칙도 합치거나 고르지 않는 두 집합을 주므로(git merge의 양쪽이 모두 남거나 오래된 사본이 남는 경우) 그 뜻이 확정적이지 않다. 쓰기는 두 개를 결코 만들지 않는다. 선언되지 않은 level L heading은 `format.section`이다. 근거: non-null 값은 언제나 쓰이므로 그 섹션이 없을 수 없고, NULL 값에는 쓸 텍스트가 없으므로 섹션이 없다.
- 값은 file 안에서 framing된다: 값이 비어 있지 않을 때에만 heading 뒤에 빈 줄이 서고, 값의 줄들이 따르며, 뒤따르는 요소가 있으면 그 앞에 빈 줄이 선다. 값은 줄 바꿈으로 시작하거나 끝나지 않으며, 양 끝에 빈 줄(space와 tab만 있는 줄, CommonMark의 빈 줄)을 담지 않는다. 그런 값의 쓰기는 `CONSTRAINT`다. 근거: framing 빈 줄은 요소를 가르므로, 줄 바꿈으로 시작하거나 끝나는 값은 framing이 바뀐 채 읽히고, 양 끝의 빈 줄은 framing 줄로 읽힌다.
- framing 빈 줄은 skeleton이다: 읽을 때 값의 양 끝에 있는 추가 빈 줄(space와 tab만 있는 줄)은 값의 일부가 아니며 `format.noncanonical`이며, 요소가 여전히 다시 읽히는 framing 빈 줄의 누락도 `format.noncanonical`이다. 다만 field list의 마지막 항목 바로 뒤에 오는 body 줄은 `format.block`이다. `flowmark canon`이 둘 다 고친다.
- 값은 최상위 setext heading을 담지 않는다: 값의 최상위 문단의 줄 바로 뒤에 `=`만 또는 `-`만 있는 줄(들여쓰기 최대 세 칸, 뒤의 공백이나 tab은 있어도 된다)은 쓰기에서 `CONSTRAINT`이며(문단 앞머리의 link reference definition은 그 문단의 줄이 아니다, [충돌 규칙](#상세-heading과-anchor)), 읽기에서는 CommonMark가 setext heading으로 읽는 곳이 heading이다. 근거: 최상위 setext 밑줄은 값의 마지막 줄을 최상위 heading으로 바꾸며, setext heading은 level 1 또는 2이고 모든 행 scope의 level L은 2 이상(`files` 행은 2, level h의 상세 섹션 안에서는 h + 1 ≥ 3)이므로 L보다 깊은 setext heading은 없다: 값의 모든 최상위 setext heading이 섹션 경계이며, 이 규칙이 그 모두를 거절한다.
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
- `codec` setting을 가진 column은 그 stage가 encode한 값을 담으며, 그 값은 column type의 readable 형태로
  쓴다 — text는 그 자리의 규칙 아래 그대로, `bytes` column은 padding이 있는 base64로 — 자기만의 형태로
  쓰지 않는다. 읽기는 encode된 값을 돌려준다. 근거: SQL parity — database는 encode된 값을 저장하고
  client가 codec을 적용한다([capability table](60-dialect.ko.md#wire)). code span은 따로 쓰는 형태가
  아니다. list 행에서 CommonMark는 backtick 묶음을 왼쪽부터 제목을 가로질러 맞추므로(§6.1), 모든 자리에서
  성립하는 형태는 type의 형태다. type에 맞지만 그 stage가 decode할 수 없는 non-NULL 값은 validator의
  `codec`이다([constraints](50-constraints.ko.md#codec-값)).
- foreign key는 link다([외래 key의 link](#외래-key의-link) 참조).
- 엔진은 수와 `true`/`false`를 orm의 `dbspec.CanonicalLiteral(t, v)`로, date, time, datetime, uuid를 `CanonicalLiteral(t, "'" + v + "'")`로 읽고, 결과에서 따옴표를 뗀다.
- `now`는 dbspec이 `datetime` 기본값에서 현재 시각을 뜻하는 keyword이며 결코 저장되는 값이 아니다: 엔진이 insert에서 그것이 뜻하는 시각을 쓴다. `now`로 쓰인 시간 값(date, time, datetime)은 그 type이 아니며(읽기에서 `type`, 쓰기에서 `CONSTRAINT`), text 값 `now`는 text다. 근거: `now`는 쓰는 시점을 가리키므로, 저장된 `now`는 읽을 때마다 다른 시각을 가리키게 된다. text는 column의 길이로 검사하며, 한 줄 자리에서는 [한 줄 자리](#한-줄-자리)의 규칙으로도 검사한다.
- 읽을 때 쓰인 형식이 canonical 형식과 다른 값은 `format.noncanonical`이다. column의 type이 아닌 값은 validator의 `type`이며, file을 읽을 수 없게 하지 않는다: 그 값을 평가하는 요청 — 값을 돌려주거나, predicate, join, 정렬, 그룹, 집계에 쓰는 요청 — 은 진단을 밝히며 `CONSTRAINT`로 실패한다. 엔진이 시작하는 쓰기(CLI, MCP, sidecar, 이력)가 column type이 아닌 값을 배정하면 `CONSTRAINT`이고, slot의 type이 아닌 orm parameter는 어떤 행도 읽기 전에 `CODEC_ENCODE`다([capability table](60-dialect.ko.md#wire)). 쓰기가 배정하지 않는 값의 `type` 진단은 행 자신의 것이든 다른 행의 것이든 commit을 실패시키지 않으며, 그 값을 commit의 제약 검사(unique key, `check`, `limit`)가 평가해야 할 때에만 `CONSTRAINT`로 진단을 밝히며 실패시킨다. 그리고 `flowmark canon`을 포함해 file을 다시 쓸 때마다 그 값의 쓴 텍스트를 바이트 그대로 보존한다. key는 이 규칙에서 값이 아니다: 키의 type이 아니거나 정식 텍스트가 아닌 key는 `format.block`이다([document model](20-document-model.ko.md#key)). 근거: 엔진이 type으로 읽지 못하는 값에는 답도 비교도 없으므로 그 값이 필요한 요청이나 검사는 답할 수 없고, 필요하지 않은 것은 영향을 받지 않는다.

- 쓸 때 수, `true`/`false`, date, time, datetime, uuid는 `CanonicalLiteral`이 주는 형식으로 쓴다(`1.5`를 `decimal(4,2)`에 넣으면 `1.50`; float는 왕복하는 가장 적은 자릿수의 고정 표기, 0은 `0`). `CanonicalLiteral`이 거절하는 값 — scale보다 많은 소수 자릿수(0을 포함), precision − scale보다 많은 정수 자릿수, precision보다 많은 소수 자릿수의 time이나 datetime — 은 엔진이 시작하는 쓰기(CLI, MCP, sidecar, 이력)에서 `CONSTRAINT`이고, orm parameter에서는 어떤 행도 읽기 전에 `CODEC_ENCODE`다([capability table](60-dialect.ko.md#wire)). bytes는 padding이 있는 표준 base64(RFC 4648 §4)로 쓰며, bytes column에서 그 밖의 텍스트를 읽으면 `type`이다. 근거: orm과의 parity.
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
- 자식 행은 부모보다 두 칸 더 들여 쓰며, 이는 표지 뒤에 space 하나로 쓴 부모의 CommonMark list 항목 내용 들여쓰기다. 들여쓰기가 `parent` column이며 다른 것이 이를 쓰지 않는다.
- list 표지 뒤의 space와 tab의 연속은 column으로 재며 tab은 다음 4의 배수까지 나아가고, CommonMark 0.31.2에서 항목의 내용이 시작하는 곳을 정한다: 한 column에서 네 column까지면 내용은 그 연속 뒤에서 시작하고, 다섯 column 이상이면 첫 space 뒤에서 시작하며 나머지가 indented code block을 연다(§5.2, 규칙 1과 2). space 하나가 정식이다. 읽기에서, 두 column에서 네 column까지의 연속은 `format.noncanonical`이다: 항목은 같은 문단을 담으므로 viewer는 같은 task 항목을 보여 주며 `flowmark canon`은 space 하나를 쓴다. 그런 행 아래 두 칸 들여 쓴 행은 `format.block`이다: CommonMark는 그것을 바깥 list의 다음 항목, 곧 형제로 읽지만 문법은 자식으로 읽기 때문이다. 표지가 그런 행의 내용이 시작하는 곳이나 그보다 최대 세 column 뒤에 서고, 그 들여쓰기에 두 칸 문법이 자리를 주지 않는(홀수 들여쓰기, 또는 level을 건너뛴) 행은 CommonMark가 읽듯 그 행의 자식이며 `format.noncanonical`이고, `flowmark canon`은 그것을 부모보다 두 칸 더 들여 쓴다. 다섯 column 이상의 연속은 `format.block`이다: CommonMark는 glyph, key, 제목, field를 code로 읽으므로 viewer는 task 항목도 inline markdown도 보여 주지 않기 때문이다. 근거: 항목의 문단이 시작하는 곳만 옮기는 구분자는 viewer가 보여 주는 것을 바꾸지 않지만, 중첩이나 보여 주는 block을 바꾸는 구분자는 행에 두 가지 읽기를 남긴다. 중첩은 CommonMark가 읽듯 읽으며, 들여쓰기는 부모 아래 두 칸일 때만 정식이다: 두 칸 문법과 CommonMark가 모두 행의 자리를 정하되 서로 다르면 file은 두 가지로 읽히므로 `format.block`이며, 따라서 구분자가 행의 부모를 조용히 바꾸는 일은 없다. CommonMark만 자리를 정하면 뜻은 확정적이며 들여쓰기만 정식이 아니다. CommonMark 0.31.2가 모든 구조와 마찬가지로 중첩의 유일한 기준이다([Readable 문서](#readable-문서)): 달리 중첩하는 구현은 그것을 정하지 않는다.
- 배치 foreign key와 `parent`는 block list 행의 field로 쓰지 않으며, 쓰면 `format.block`이다.

## Table 행

- table의 header 행은 저장되는 scalar column을 이름 붙이며 `| <key column> | <column> | … |`로 쓴다. 저장되는 모든 scalar column은 header를
  가지며, header가 없거나 어떤 column도 이름 붙이지 않는 header는 `format.block`이다. key는 첫 cell이며 key column 이름의
  header 아래에 있다. 저장되는 scalar column은 column 순서대로 그 뒤를 잇는다. GFM이 이 column들의 header 행으로 읽되 다른 모양인 줄은 `format.noncanonical`이다: 들여쓰기가 최대 세 칸이고 첫 문자 앞에 tab이 없으며, 양 끝의 `|`는 선택이고(column 하나인 header는 `|`가 없어도 된다), cell 주위에 space와 tab이, 줄 끝에 space, tab, 수직 탭(U+000B), 폼 피드(U+000C) 중 어느 것이든 올 수 있다. 네 column 이상 들여 쓴 줄은 `format.block`이며, 들여쓰기의 tab은 다음 4의 배수까지 나아간다. 근거: GFM은 header를 양 끝 pipe가 선택이고 그 delimiter 행이 허용하는 공백으로 끝나는 행으로 읽으며(GFM 0.29, Tables), 그것은 문단 줄 위에 있고, CommonMark 0.31.2는 문단 줄에서 최대 세 칸의 들여쓰기를 떼어내지만(§4.8) 네 column은 indented code block을 연다(§4.4). tab은 다음 tab stop까지 나아간다(§2.2). 그런 header는 모두 같은 table을 만들므로 `flowmark canon`이 고친다. delimiter 행에서처럼 GFM 0.29가 table의 기준이다. 구조는 spec으로 판단하기 때문이다([Readable 문서](#readable-문서)): VS Code의 markdown-it은 `|`가 없는 header에서 table을 읽지 않으며, 그 일탈은 그것을 정하지 않는다.
- table의 delimiter 행은 column마다 `---`이며 `| --- | --- |`로 쓴다. GFM이 header의 delimiter 행으로 읽는 줄은 `format.noncanonical`이다: 들여쓰기가 최대 세 칸이고 첫 문자 앞에 tab이 없으며, cell들이 `|`로 나뉘며, 각 cell은 하나 이상의 하이픈이고 양 끝에 최대 하나의 콜론이 있으며 그 주위에는 space, tab, 수직 탭(U+000B), 폼 피드(U+000C) 중 어느 것이든 올 수 있고, 줄 양 끝의 `|`는 선택이며 그 뒤에는 이 네 문자 중 어느 것이든 올 수 있고, cell 수는 header와 같다. 다만 앞에 `|`가 없는 줄이 CommonMark가 먼저 다른 block으로 읽는 경우는 예외다: 하이픈만 있고 뒤에 space나 tab만 붙은 줄(setext 밑줄이며 header를 heading으로 만든다), 또는 들여쓰기 뒤 첫 문자가 `-`이고 그 뒤에 space나 tab이 오는 줄(list 항목). 따라서 정렬 콜론은 `format.noncanonical`이다. 그 자리의 다른 줄은 `format.block`이다. 근거: delimiter 행은 skeleton이며 어떤 column도 정렬을 말하지 않으므로, 쓰기가 정렬을 보존할 수 없고 형식은 하나만 정식이다. GFM의 delimiter 행은 cell 주위에 이 네 공백 문자를 허용하므로, 그런 행은 모두 같은 table로 읽히며 `flowmark canon`이 고친다. delimiter 행의 공백은 skeleton이며 한 줄 자리가 아니다. 들여쓰기의 tab은 네 번째 column에 닿으므로 CommonMark는 그 줄을 들여 쓴 줄로 읽으며, 그것은 header의 문단을 잇고 table을 열지 않는다(§2.2, §4.4). GFM이 delimiter 행으로 읽지 않는 줄은 table을 만들지 않으며, viewer는 header와 행들을 문단 텍스트로 보여 준다.
- 선언된 title은 column의 header 아래 cell이며 column 순서를 따른다.
- block table에서 배치 foreign key는 결코 cell이 아니며, `parent`는 cell이다. 같은 block의 두 행이 공유하는 key를 가리키는 `parent` cell은 `format.block`이다: 부모를 배정할 수 없기 때문이다. 서로 다른 block의 두 행이 공유하는 key는 block이 배정한다.
- cell은 text를 있는 그대로 담는다. 빈 cell은 NULL이고, 빈 텍스트는 `""`로 쓴다. 모든 non-null 저장
  scalar column은 값을 갖는다.
- 행은 양 끝에 `|`를 두고 쓴다. 읽기에서, 마지막 `|`가 없는 행은 GFM이 읽듯 읽으며 — 마지막 cell은 줄 끝까지 이어지고, 마지막 `\|`는 그 cell의 `|`다 — `format.noncanonical`이다. 근거: GFM 행의 양 끝 pipe는 선택이므로 그 행은 같은 cell을 담으며, `flowmark canon`이 `|`를 쓴다.
- `"`로 시작하는 cell 값은 `"…"` 안에 `\"`와 `\\`를 escape로 써서 쓴다. table cell에서 값의 각 `|`는 `\|`로 쓰고 다른 `\`는 더하지 않는다. 읽을 때 `\` 바로 뒤에 오는 `|`는 그 `\` 앞에 무엇이 있든 cell 경계가 아니며, 그 `\` 하나를 뺀다. 인용형을 먼저 적용하고 pipe escape를 마지막에 적용하므로, 읽을 때는 pipe escape를 먼저 뺀다. 근거: GFM은 `\` 뒤의 모든 `|`를 cell 텍스트로 읽고 inline 파싱 전에 그 `\`를 뺀다.

## 한 줄 자리

제목(list 행, H1, cell 안의 제목), list field, field list의 값, table cell은 한 줄 자리다. 한 줄 값은 쓴 그대로의 inline markdown이며, 이 절이 정한 경우를 빼고는 escape하지 않는다. 이 절에서 공백(white space)은 Unicode White_Space 속성을 가진 문자다. 보이지 않는 문자(invisible character)는 U+1680을 뺀 공백, Unicode 범주 Cc(제어 문자), Prepended_Concatenation_Mark(U+0600–U+0605, U+06DD, U+070F, U+0890–U+0891, U+08E2, U+110BD, U+110CD)를 뺀 Cf(형식 문자, 예컨대 U+FEFF), Default_Ignorable_Code_Point(예컨대 U+3164, U+115F, U+034F, variation selector)의 문자다. Unicode 속성은 엔진이 빌드되는 Go unicode 패키지의 Unicode 17.0.0 값이다. 이 절의 규칙은 값의 양 끝에 있는 보이지 않는 문자를 거절하며, 값 안의 문자는 끝이 아니다. 구분자 안에서 정식 공백은 space(U+0020)뿐이다. 근거: 그런 문자는 글리프 없이 그려지므로 독자는 볼 수 없다. U+1680과 prepended concatenation mark는 글리프가 있어 텍스트다. 양 끝의 보이지 않는 문자를 모두 거절하면 값의 텍스트와 독자가 보는 것이 같다. CommonMark 0.31.2는 문단과 ATX heading 내용의 양 끝에서 space와 tab을 떼어내며, 이 둘은 이 집합의 일부다.

- 쓰기에서, 그 자리에 줄바꿈이나 CR을 가진 값, 앞이나 뒤에 보이지 않는 문자를 가진 값, 또는 `<` 뒤에 `!`, `/`, 글자가 오는 값은 `CONSTRAINT`다. 줄바꿈은 한 줄에 쓸 수 없고, raw HTML은 값을 숨기거나 바꾸기 때문이다.
- 읽기에서, 렌더링이 inline markdown을 넘어 텍스트와 다르게 보이는 한 줄 값은 `format.block`이며, 정확히 다음이다: 인용된 list field나 인용된 cell(`"…"`)의 따옴표 안 양 끝에 보이지 않는 문자가 있는 것, 또는 `<` 뒤에 `!`, `/`, 글자가 오는 값. 읽기에서, CommonMark가 줄 내용의 끝에서(또는 GFM이 cell의 끝에서) 떼어내는 자리에 있는, 한 줄 자리의 끝의 space나 tab은 `format.noncanonical`이다. list 행의 key(list 표지 뒤의 연속은 제외한다), 제목, field 앞의 space와 tab의 연속은 구분자이며, 그 연속이 space 하나가 아니면, field 앞에서 list 제목을 끝내는 tab을 포함해, `format.noncanonical`이다. 그 이유는 field 파서가 space와 tab을 똑같이 나누고, 쓰기 규칙이 둘 중 하나로 끝나는 제목을 거절하므로, 그 연속은 제목 텍스트가 아니기 때문이다. list 표지 뒤의 두 개에서 네 개까지의 space 연속은 `format.noncanonical`이고 다섯 개 이상의 연속은 `format.block`이다([List 행](#list-행)). list 표지 뒤의 연속은 column으로 재며 tab은 다음 4의 배수까지 나아가고(CommonMark 0.31.2 §2.2), 그 너비의 space 연속으로 판정하며([List 행](#list-행)), 그 안의 tab은 `format.noncanonical`이다. 제목의 token 사이의 공백은 제목 텍스트이지 구분자가 아니다. 끝에 있는 그 밖의 보이지 않는 문자는 쓰기 규칙이 다시 만들 수 없으므로 `format.block`이다. 각 자리는 하나의 정식 구분자를 가진다(H1 표지 뒤 `# `, field list column 뒤 `: `, 빈 텍스트는 아무것도 없음, list 표지 뒤와, list 행의 glyph, key, 제목, 각 field 사이에 space 하나, cell은 `| v |`). 그 밖의 space나 tab은 `format.noncanonical`이다.
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
- 행의 유도된 target을 바꾸는 쓰기 — 행의 상세 섹션을 더하거나 빼거나, dangling 참조가 가리키는 행을
  insert하는 쓰기 — 는 같은 transaction에서 그 행을 가리키는 모든 link를 다시 쓴다.
- 유도된 것과 다른 target은 `format.noncanonical`이다. 어떤 행도 가리키지 않는 key는 dangling 참조다: validator가 `reference.dangling`으로 보고하고, 이를 새로 만드는 쓰기는 `FOREIGN_KEY`이며, file을 읽을 수 없게 하지 않는다. 쓴 target은 그대로 간직한다: 유도할 target이 없으므로 target 검사 없이 읽고 읽은 그대로 쓴다. 다만 block이나 document 행의 `parent`가 그 block의 어떤 행도 가리키지 않으면 `format.block`이다. 근거: 없는 행에는 유도할 target이 없고, 쓰기가 바꾸지 않은 target은 그 바이트를 유지해야 하며, 한 block 안의 계층은 저장 순서를 가지려면 완전해야 한다. key는 결코 갱신되지 않으므로([document model](20-document-model.ko.md#key)) target은 link 아래에서 움직이지 않는다.
- link의 유도된 target은 그 key가 가리키는 한 행의 target이다. block이나 document 행의 `parent`는 자기 block의 행을 가리키므로([문서 순서](#문서-순서)), 다른 block의 행들이 그 key를 공유하더라도 그 target은 그 행에서 유도된다(자기 block의 두 행이 그 key를 공유하면 그 cell은 `format.block`이다, [Table 행](#table-행)). 그 밖의 foreign key는, 참조된 table의 두 행 이상이 그 key를 바이트 단위로 공유하면 어떤 한 행도 가리키지 않는다: 유도된 target이 없으며, dangling 참조처럼 target 검사 없이 읽고 읽은 그대로 쓴다. 근거: target은 참조된 행의 함수이고, 두 행이 공유하는 key는 어떤 규칙도 그 사이에서 고르지 않는 두 target을 준다. 행이나 file의 순서로 고르면 file의 진단이 그 순서에 달리게 된다. 그 table은 `key.duplicate`로 사용할 수 없으므로([사용할 수 없는 table](#진단)) 어떤 쓰기도 그런 link를 다시 쓰지 않고 `flowmark canon`은 그 target의 유도를 거절한다([operations](40-ops.ko.md#canon-연산)). key가 고쳐지면 target이 유도되고 검사된다. 대소문자를 무시할 때만 같은 key들은 바이트 단위로 각각 한 행을 가리키므로, 그 어느 쪽으로의 link든 유도된 target을 가진다.
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
- `format.*` 진단이 있는 file을 다시 쓰는 logical write는 `CONSTRAINT`다, 면제를 제외하고. 다른 행의 validator 진단(`type`, `key.duplicate`, `reference.dangling`, `check`, `codec`, `view`, `order`, `state.transition`, `state.required`, `state.limit`, `history.gap`, `history.chain`, 여러 `files` file에 걸친 `parent.cycle`)은 commit을 실패시키지 않되, `type` 진단은 commit의 제약 검사가 그 값을 평가해야 할 때 commit을 실패시키고, 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([사용할 수 없는 table](#진단)); `format.*` 진단은 다시 쓰기 거절과 사용할 수 없는 table 상태([사용할 수 없는 table](#진단))가 정하는 경우에만 commit을 실패시킨다. file을 다시 써야 하는 commit — 행 자신의 file, 유도된 target이 바뀌는 행(상세 섹션이 생기거나 사라지거나, insert가 dangling link에 그 행을 주는 행)으로의 link를 가진 file, `order`의 형제 — 은 그 file이 `format.*` 진단을 가지면 거절한다: `CONSTRAINT`이며 file과 진단을 밝히고, `format.noncanonical`이면 `flowmark canon`을 밝힌다. 세 commit은 면제된다: 대상에 대한 `flowmark canon`([operations](40-ops.ko.md#canon-연산)), 추가된 column이 일으킨 진단에 대한 add-column migration([migration](80-migration.ko.md)) — 그 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다 —, `files` 행의 file 전체를 지우는 delete는 rewrite 거절에서만 면제되는데, file을 지우는 것은 그 file의 바이트를 다시 쓰지 않고 거절이 지키는 것이 바로 그 바이트이기 때문이다. restrict 검사는 그 행을 참조할 수 있는 모든 table의 행을 여전히 필요로 하며(자신의 table을 `parent`로 하는 것, 자식 block과 이력 행, 그 행으로의 foreign key를 가진 모든 table), 그 중 하나라도 사용할 수 없으면 그 delete는 `CONSTRAINT`다. install이 쓰는 `.gitattributes`가 없으면 Windows `autocrlf` checkout은 모든 값을 읽지만 `flowmark canon` 전까지 위의 면제를 뺀 기존 문서를 다시 쓰는 모든 쓰기를 거절한다. 근거: 쓰기는 사용자가 쓰지 않은 바이트를 바꾸면 안 되며, migration은 별도의 명시적 transaction이다.
- `flowmark install`은 `<corpus>/.gitattributes`에 `*.flowmark.md text eol=lf` 줄을 더하며, 파일이 없으면 만들고 다른 줄은 유지한다. 그래서 git checkout은 모든 플랫폼에서 LF를 유지한다([schema](30-schema.ko.md#install)).

## 진단

위의 모든 읽기 규칙과 walk는 `{file, line, column, rule, message}`를 보고하며, `column`은 규칙이 이름 붙인 요소(glyph, key, 제목, field token, cell 내용, heading 텍스트)의 첫 글자의 1부터 세는 code point 열이다. 줄 전체의 결함은 열 1이다. `format.encoding`이 잘못된 UTF-8 byte에 대한 것이면 그 byte에 있으며, 열은 그 줄에서 그 앞의 code point 수에 1을 더한 것이다: byte는 한 자리를 차지한다. file 전체의 결함이나 텍스트가 없는 corpus 항목(`corpus.walk` 항목, 폴더, symlink 포함)의 결함은 줄 1, 열 1이다. `catalog` 진단도 code point를 센다. 근거: 열은 규칙이 이름 붙인 텍스트의 자리를 정하므로, 글자가 아닌 byte는 디코더가 읽는 자리를 가진다. orm의 dbspec은 lexer에서 code point 단위로 열을 세며(`splitSource`, `lexLine`, `dbspec.go`의 `Diagnostic`), 문서 전체의 결함(크기 한도, byte order mark)은 줄 1, 열 1이다. 읽기 규칙 이름은 `format.noncanonical`, `format.section`, `format.block`, `format.encoding`, `parent.cycle`이고, walk의 규칙은 `corpus.walk`이며, 그 내용은 [document model](20-document-model.ko.md)에 적는다.

- **읽기와 쓰기의 결함.** 읽기에서 다음은 `format.block`이다: 없는 non-null field나 section; 렌더링이 inline markdown을 넘어 텍스트와 다르게 보이는 한 줄 값([한 줄 자리](#한-줄-자리) 참조); `-` list 항목으로 시작하는 `files` body; field list 뒤에서 첫 줄이 두 column 이상(space 또는 tab) 들여쓴 `files` body. 같은 결함은 쓰기에서 `CONSTRAINT`다. 읽기에서, CommonMark가 줄 내용의 끝에서(또는 GFM이 cell의 끝에서) 떼어내는 자리에 있는, 한 줄 자리의 끝의 space나 tab은 `format.noncanonical`이다. list 행의 key(list 표지 뒤의 연속은 제외한다), 제목, field 앞의 space와 tab의 연속은 구분자이며, 그 연속이 space 하나가 아니면, field 앞에서 list 제목을 끝내는 tab을 포함해, `format.noncanonical`이다. 그 이유는 field 파서가 space와 tab을 똑같이 나누고, 쓰기 규칙이 둘 중 하나로 끝나는 제목을 거절하므로, 그 연속은 제목 텍스트가 아니기 때문이다. list 표지 뒤의 두 개에서 네 개까지의 space 연속은 `format.noncanonical`이고 다섯 개 이상의 연속은 `format.block`이다([List 행](#list-행)). list 표지 뒤의 연속은 column으로 재며 tab은 다음 4의 배수까지 나아가고(CommonMark 0.31.2 §2.2), 그 너비의 space 연속으로 판정하며([List 행](#list-행)), 그 안의 tab은 `format.noncanonical`이다. 제목의 token 사이의 공백은 제목 텍스트이지 구분자가 아니다. 끝에 있는 그 밖의 보이지 않는 문자는 `format.block`이다. 근거: 읽기는 문서가 담은 것을 보고하고, 쓰기는 다시 읽을 수 없는 것을 만들어 내지 않는다.
- **쓰기의 결함.** 거절된 쓰기는 바이트를 쓰지 않으므로, 결함은 file이 아니라 요청에 위치한다: 각각 `{table, key, column, rule, message}`를 보고하며, `table`과 `key`는 규칙이 거절하는 행을, `column`은 규칙이 이름 붙인 값의 column을 가리킨다 — key 결함이면 key column, 제목, field, cell이나 `markdown` column, 자식 table(그 block heading에 대해), `parent`, `order` — 행 전체의 결함이면 비어 있다. 그 table의 column도 자식 table도 아닌 이름 아래 준 값은 그 이름에서 거절되며, 자식 block의 행들은 그 block의 이름에서 거절된다. 한 쓰기에 준 행들이 key를 바이트 단위로 공유하면, 그런 행의 각 결함은 준 행들 가운데 0부터 세는 index도 가지며, 중복 결함은 첫 행 다음의 그런 행마다 있고 첫 행을 밝힌다. 대소문자를 무시하면 같은 key인 두 행 가운데, 쓰기가 삽입하는 행 쪽에 중복 결함이 있고 다른 행을 밝힌다. 쓰기가 둘 다 삽입하면 byte 순서에서 더 큰 key를 가진 쪽이다. 행은 그 key column에 최대 하나의 중복 결함을 가진다. 그 행보다 먼저 준 행이나 요청이 주지 않은 table의 행이 그 key를 바이트 단위로 공유하면, 그 결함은 그보다 먼저 준 그런 행 가운데 첫 행을, 그런 행이 없으면 table의 그 행을 밝힌다. 그렇지 않으면 준 행이든 table의 행이든, 그 key와 대소문자를 무시할 때만 같은 key를 가지며 그 쌍이 결함을 이 행에 두는 행들 가운데 byte 순서에서 가장 작은 key를 가진 행을 밝히고, 준 행 여럿이 그 key를 가지면 처음 준 행을 밝힌다. 근거: 결함은 자리를 하나 더하며, 한 `{table, key, index, column}`의 두 번째 결함은 아무 자리도 더하지 않는다. 바이트 단위 결함을 먼저 밝히는 것은, 그것이 쓰기가 어느 행을 도입하든 성립하고 요청 안의 자리를 밝히는 반면, 대소문자만 다른 결함의 자리는 쓰기가 어느 행을 삽입하는지에 달리기 때문이다. 한 쓰기의 key 검사 — 바이트 단위와 대소문자를 무시한 것 모두, 요청 안과 table의 다른 행에 대해 — 는 어떤 file도 렌더링하기 전에 실행되며, 그것이 거절한 쓰기는 어떤 file도 렌더링하지 않는다. 따라서 이 자리들은 요청의 것이고 그 index는 요청에서 행의 위치다. block이나 document 그룹의 형제 순서 결함은 값이 없거나, 1..m 밖이거나, 형제와 공유되는 그룹의 각 행에, column `order`에 있다(`files` 그룹은 engine이 고르는 rank를 담으므로 그 쓰기에는 이런 결함이 없다([document model](20-document-model.ko.md#순서))). column의 type이 아닌 값은 type 결함뿐이며, m에 세어지고, 1..m 밖도 공유도 아니다. 부모 순환은 순환 위의 각 행에 있으며 column `parent`다. 없는 행을 부모로 가지거나 순환 아래에 있는 행은 자기 결함만 가진다. 다시 써야 하는 file이 읽기 진단을 가지므로 거절된 쓰기는, 그 file을 밝히고 그 진단의 `{file, line, column}`을 싣는다. 근거: 거절된 쓰기가 만들 바이트는 존재하지 않으므로 그 줄들은 열 수 없고, 그 자리는 거절된 텍스트와 묶인 행이 어떻게 놓이는지에 따라 달라질 것이다. 요청은 행을 table과 key로, 값을 column으로 이름 붙이므로, 이 자리는 요청만의 함수다. 이는 SQL 엔진이 위반의 table, 제약, column을 이름 붙이는 것과 같다. 거절된 쓰기가 여러 규칙의 결함을 가지면 그 code는 첫 결함의 code이며, 모든 결함은 이 순서로 보고한다: 행은 요청 순서(insert는 준 순서, update나 delete는 저장 순서)로; 한 행 안에서는 그 값 결함 — key 모양, type, check, codec, view, required, title, single-line, order, limit, 렌더링된 file이 필요 없는 것 — 이 key 결함보다 먼저이며, 두 key는 A–Z를 접었을 때 같으면 대소문자를 무시하고 같다([document model](20-document-model.ko.md#key)); 렌더링된 file이 필요한 결함(anchor 충돌, 읽기 진단을 가진 file의 다시 쓰기 거절)은 key나 값 결함이 없을 때만; `FOREIGN_KEY`는 어떤 행도 다른 결함을 갖지 않을 때만. 근거: SQL parity — MySQL, PostgreSQL, SQLite는 행의 CHECK 결함을 key 결함보다 먼저 보고하며, PostgreSQL과 SQLite는 foreign key를 문장 끝에서 검사한다. key와 값 검사는 모든 렌더링에 앞서며, 그것이 거절한 쓰기는 file을 렌더링하지 않는다: 그 쓰기는 file이 무엇을 담든 거절되며, codec은 값 검사가 거절한 값을 다시 거절할 것이기 때문이다.
- **validator 규칙.** column의 type이 아닌 값은 `type`이고, dangling 외래 key는 `reference.dangling`이며, 대소문자를 무시하고 같은 table의 다른 key와 같은 key는 `key.duplicate`다. 행에 대해 false인 check는 `check`이고([constraints](50-constraints.ko.md#check)), 앞 행을 잇지 않거나, 선언되지 않은 쌍을 기록하거나, 필수 값이 없는 이력 행은 `history.chain`, `state.transition`, `state.required`다([constraints](50-constraints.ko.md#엔진-밖에서-바뀐-상태)). type에 맞지만 codec stage가 decode할 수 없는 값은 `codec`이고([constraints](50-constraints.ko.md#codec-값)), 유효한 view를 담지 않은 view 행은 `view`다([constraints](50-constraints.ko.md#view)). validator는 이것들을 corpus 전체에서 보고한다. 이것들은 file을 읽을 수 없게 하지 않는다. 다만 block이나 document 행의 `parent`가 그 block의 어떤 행도 가리키지 않으면 `format.block`이다([canonical format](#외래-key의-link)), 그리고 같은 block의 두 행이 공유하는 key를 가리키는 `parent` cell도 `format.block`이다([canonical format](#table-행)), 그리고 같은 block의 두 행이 공유하는 key를 가리키는 상세 섹션이나 file 안에서 같은 heading 텍스트를 가진 두 상세 섹션도 `format.block`이다([문서 순서](#문서-순서), [상세 heading과 anchor](#상세-heading과-anchor)). 근거: file의 읽을 수 있음은 다른 file에 의존하면 안 된다. 그렇지 않으면 부모 file을 지울 때 자식이 읽을 수도 고칠 수도 없게 된다. column의 type이 아닌 값은 엔진이 시작하는 쓰기에서 `CONSTRAINT`이고 orm parameter에서 `CODEC_ENCODE`다([capability table](60-dialect.ko.md#wire)). dangling 외래 key나 중복 key를 새로 만드는 쓰기는 각각 `FOREIGN_KEY`, `DUPLICATE_KEY`다. 커밋 거절은 [Encoding과 줄 구조](#encoding과-줄-구조)에 적는다. 열기, introspection, `flowmark validate`, `flowmark canon`은 행에 대한 요청이 아니므로 사용할 수 없는 table에서 결코 실패하지 않으며, canon의 링크 유도 거절은 예외다([operations](40-ops.ko.md#canon-연산)). validator는 모든 file의 `format.*`와 `parent.cycle` 진단과, `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 없는 file 안에서 읽을 수 있는 모든 행의 validator 규칙을 보고한다. `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가진 table의 완전한 행을 필요로 하는 규칙 — 그 table로의 `reference.dangling`, `key.duplicate`, 그 안의 `order`와 `state.limit` — 은 그 table에 대해 보고하지 않으며, file 자신의 진단이 그것을 대신한다. 여러 `files` file에 걸친 validator의 `parent.cycle`은 읽을 수 있는 행 사이에서 보고한다. 순환은 그 행들 자신의 `parent` 값으로 만들어지며 읽지 못한 어떤 행도 그것을 풀 수 없기 때문이다. 바이트 단위로 같은 key 때문에만 사용할 수 없는 table은 모든 규칙이 보고된다. key가 그 column의 type 값이 아닌 외래 key는 `type`뿐이며 결코 `reference.dangling`이 아니다. 근거: 결함 하나에 보고 하나 — 그런 key는 column이 가질 수 있는 값이 아니므로, 그것이 행을 가리키는지는 묻지 않는다. 그런 key를 쓰는 쓰기는 `CONSTRAINT`(orm parameter이면 `CODEC_ENCODE`)이며 결코 `FOREIGN_KEY`가 아니다.
- **Corpus 순서**는 행들의 자리의 순서다: file의 corpus 상대 경로(`/` 구분)의 byte 순서, 다음으로 줄, 다음으로 column. 읽을 수 있는 모든 corpus에서 전순서다. tree가 온전하지 않은 `files` table에서 corpus 순서가 완성하는 저장 순서는 [document model](20-document-model.ko.md#순서)이 정한다.
- **사용할 수 없는 table.** table은 `format.block`, `format.section`, `format.encoding`이나 그 행 사이의 `parent.cycle`이 있는 file을 가지거나 바이트 단위로 같은 key를 가진 두 행이 있으면 사용할 수 없다. 이때 요청은 두 번째 행의 file과 `key.duplicate`를 밝힌다. table은 그 storage가 이름을 정한 자리의 모든 corpus 문서를 가진다: `files` table은 모든 `<table>/<name>.flowmark.md`, `document` table은 `<table>.flowmark.md`, block table은 placement parent table이 가진 모든 file을 재귀적으로 가지므로 block으로 저장된 이력 table은 state table과 함께 사용할 수 없다. table은 `corpus.walk` 항목이 그 자리에, 또는 대소문자를 무시하고 그 자리와 같은 이름에 서 있는 동안에도 사용할 수 없다 — 읽을 수 없는 항목, `files` table의 자리에 있는 symlink나 폴더가 아닌 항목, 또는 다른 대소문자의 suffix, 폴더, document 이름. 근거: 없는 폴더는 빈 table이지만 그 자리의 항목은 폴더도 없음도 아니며, 엔진은 자기가 소유하지 않은 항목의 이름을 바꾸거나 지우지 않으므로 그 table의 행을 읽거나 쓸 수 없다. 대소문자를 무시하고도 `.flowmark.md`로 끝나지 않는 file은 database 밖이며 table을 결코 사용할 수 없게 하지 않는다. 그 행 중 어떤 것이든 필요로 하는 모든 요청 — 키로 하는 point 조회를 포함한 읽기, table의 insert·update·delete, 그 위의 join, relation, subquery, 링크가 그 안에 있을 수 있을 때 유도된 target이 바뀌는 행 — 상세 섹션이 생기거나 사라지거나, insert가 dangling 참조에 그 행을 주는 행 — 으로의 링크 다시 쓰기, 그리고 그 위의 모든 제약 검사(primary나 unique key, 참조, 링크 target, `limit` 개수, `order` 형제 집합, `check`) — 은 file과 진단을 밝히며 `CONSTRAINT`로 실패한다. 요청은 다른 file들의 행만으로 결코 답하지 않는다. 키의 정식 텍스트가 아닌 이름을 가진 `files` file은 `format.block`이므로 그 table도 사용할 수 없다. 근거: 엔진이 읽지 못하는 행에 대한 답이나 검사는 그 사실을 말하지 않고 틀리게 되며, 요청은 바이트 단위로 같은 key를 가진 두 행 중 하나를 가리킬 수 없다. 엔진은 사용할 수 없는 table에 `flowmark canon`이 file에 하는 것 외에는 아무것도 쓰지 않는다([operations](40-ops.ko.md#canon-연산)): 사람이 corpus를 편집해 그 table을 사용할 수 없게 만드는 행이나 file을 고치고, 이름을 바꾸거나 지우면, 다음 읽기에서 table은 다시 사용할 수 있다. 근거: 요청은 바이트 단위로 같은 key를 가진 두 행 중 하나를 가리킬 수 없고, 엔진이 읽지 못하는 행은 그것을 통해 지울 수 없다.
