<!-- doc-id: spec-document-model -->
<!-- source-sha256: 92aeb907bed79ef58f42c1a24a3fade3797d73533e6d7eef842a65e033b846c8 -->
# 문서 모델

[English](20-document-model.md)

이 spec은 corpus가 table과 행이 되는 방식을 고정한다. 대응은 전체적이다: corpus 문서의 모든 byte는
모델링된 자리 하나에 속하거나 parser가 보고한다. byte 형식은 [canonical format](10-canonical-format.md)에,
선언은 [schema](30-schema.md)에 있다.

## Corpus root

- corpus root는 command나 server에 준 directory다. `dbspec/*.dbs` — catalog — 와 `.flowmark/`를 담는다.
  위로 거슬러 찾지 않는다.
- 이 repository는 corpus가 아니다: 자기 checklist는 kit이 추적하는 문서다.

## Corpus 문서와 walk

- corpus 문서는 이름이 `.flowmark.md`로 끝나는 file이며, corpus root 아래에 있고 `dbspec/`, `.flowmark/`,
  숨은 항목 밖에 있다. 이름이 database의 일부가 되는 유일한 기준이다. 그 밖의 file은 corpus 밖이며
  진단을 내지 않는다. 다만 대소문자를 무시하고 suffix가 `.flowmark.md`인 이름은 `corpus.walk`이며, `files` table의 자리에 있는 폴더가 아닌 항목(아래의 symlink 규칙)도 그렇다.
- walk는 모든 `*.flowmark.md` file을 읽는다. place가 table 어느 것과도 맞지 않는 file — `document` table의
  `<table>.flowmark.md`도 아니고 `files` table의 `<table>/<key>.flowmark.md`도 아닌 것 — 은 `corpus.walk`
  진단이며, 읽을 수 없는 `.flowmark.md` 항목도 그렇다.
- 이름은 byte 단위로 비교한다. `.flowmark.md` 접미사, table 폴더, document file 이름이 다른 대소문자를
  가지면 `corpus.walk`다.
- key 모양을 깨는 file 이름은 읽을 때 `format.block`이다.
- walk는 symlink를 따라가지 않는다. 이름이 `.flowmark.md`로 끝나는 symlink와, `files` table 이름을 가진
  폴더가 아닌 항목(일반 file이나 가리키는 곳이 무엇이든 symlink)은 `corpus.walk`이며, 그 밖의 symlink는
  database 밖이다. 근거: table의 자리는 그 이름이므로, 그 이름을 가진 file은 database 밖이 아니라 table의 자리에 있으며
  그 table은 사용할 수 없다([canonical format](10-canonical-format.ko.md#진단)).
- hard link는 플랫폼에 관계없이 file과 구별할 수 없으므로 규칙이 아니다(multi-platform 요구). 엔진은
  document를 temp file과 rename으로 쓰므로, hard link된 document는 첫 쓰기에서 unlink된다.
- `dbspec/`에는 `.dbs` 집합만 있다. 그 밖의 file은 catalog 진단이다([schema](30-schema.md)).

## 저장

각 table은 정확히 하나의 저장을 선언한다. 없는 table은 schema 오류이며 기본값을 추측하지 않는다.

- **`store files`** — 행마다 file 하나: corpus root 아래 `<table>/<key>.flowmark.md`. file 이름이 key다.
  key는 결코 갱신되지 않는다: primary key를 배정하는 update는 모든 database에서 orm의 규칙인
  `IR_INVALID`이므로, 쓰기가 file 이름을 바꾸는 일은 없다.
- **`store document list`, `store document table`** — 모든 행이 corpus root의 file
  `<table>.flowmark.md` 하나에 있다.
- **`store block <fk> list`, `store block <fk> table`** — 이름 붙은 foreign key가 가리키는 부모 행의
  scope 안의 행. 그 block 섹션은 자식 table 이름을 가진 heading이며, list나 table이 다음 줄에 선다([canonical format](10-canonical-format.md#document-order)).

- 없는 table 폴더(`files`)나 없는 document 파일(`document`)은 빈 table이다: 행이 0개이고 진단은 없다. H1 줄이 없는 document file은 `format.noncanonical`이다. 근거: git은 빈 폴더를 보존하지 않으므로 checkout에는 행이 없는 table의 폴더가 없을 수 있으며, 없는 폴더나 파일은 행이 없고 빈 폴더나 `# <table>`만 가진 document file과 같으므로 그 없음은 0행의 한 형식이지 corpus의 결함이 아니다.
- table이 이름을 정하지 않은 자리의 `.flowmark.md` file은 `corpus.walk`다. 없는 table을 대신하지 않는다.
- insert는 폴더나 파일을 만든다. `document` table의 마지막 행을 지우면 file은 H1 줄 `# <table>`과 LF 하나를 남긴다.

aggregate냐 split이냐는 저장 선언이다. 논리 관계는 모든 저장에서 같으므로 같은 행을 어느 쪽으로든
저장할 수 있다.

## 시간 shard

`store` setting에 `shard <column> <period>`를 가진 `files` 또는 `document` table은 기간마다 행을 따로
저장한다. column은 non-null `date` 또는 `datetime`이고 period는 `year`, `month`, `day`다(setting과 그
오류는 orm의 것이다, `T119`). 소유하는 행이 없고 시간에 따라 쌓이는 table — 이벤트 로그, 시간 기록 — 은
그렇지 않으면 파일 하나(`document`)나 folder 하나(`files`)가 끝없이 커진다.

- **기간 이름.** 행의 기간 이름은 column 값을 grain에서 자른 것이다: `2026`, `2026-10`, `2026-10-11`.
  `datetime`은 저장된 UTC 값으로 세고 `date`는 쓰인 대로다. 12를 넘는 달처럼 날짜가 아닌 값에는 기간이
  없으며, 쓰기는 그 행을 거절하고 읽기는 `type`으로 보고한다.
- **경로.** 샤드된 `files` table은 행을 `<table>/<period>/<key>.flowmark.md`에, 샤드된 `document`
  table은 한 기간의 행을 `<table>/<period>.flowmark.md`에 저장하며 그 H1 줄은 `# <table>`이다. 행이 없는
  기간에는 파일도 folder도 없다. 빈 table에 없는 것과 같다.
- **walk.** 샤드된 table의 folder는 기간의 folder(files)나 문서(document)를 shard의 grain으로 이름 붙여
  담고, 그 밖의 문서는 담지 않는다. 샤드된 `files` table의 folder 바로 아래 문서, 다른 grain이나 기간이
  아닌 이름의 folder와 문서, 기간 folder 아래의 문서는 `corpus.walk`다.
- **저장 순서.** shard는 행이 서는 곳을 바꿀 뿐 table이 읽히는 순서를 바꾸지 않는다. 논리 관계는 어느
  저장에서나 같기 때문이다. `files` table은 기간과 무관하게 자기 순서(spec 20 순서, orm이 샤드된 table에
  계층과 order column을 허용하지 않으므로 key 순서)를 유지한다. `document` table은 기간을 오름차순으로,
  각 문서를 자기 순서로 읽는다.
- **Key.** key와 `identity` 값은 모든 기간에 걸쳐 유일하다. 모든 기간의 행이 table
  하나이며 엔진이 메모리에 유지하는 corpus에서 읽고, corpus 옆에 index 파일을 두지 않는다. 근거: table의
  key 검사는 table의 행을 읽고, shard는 행이 어디에 서는지를 바꿀 뿐 table이 어떤 행을 갖는지는 바꾸지
  않는다.
- **다른 기간의 파일에 있는 행.** column 값이 자기를 담은 파일의 기간과 다른 기간을 가리키는 행은
  validator의 행 위치 `shard.period`이고, 그 행의 다음 쓰기가 기간의 파일로 옮긴다.
- **column 갱신**은 행을 새 기간의 파일로 옮기고, 같은 transaction에서 그 행으로 링크한 모든 파일을 다시
  쓴다. 샤드된 행으로의 링크는 그 기간의 경로를 담기 때문이다. 근거: 링크는 행이 서는 곳에서 파생되므로
  (spec 10) 이동은 링크를 바꾼다.
- **절 변경**은 table의 shard를 더하거나 뺀다. 저장 변경이다(spec 80).
- **방식 하나와 적용 범위.** shard 방식은 시간 shard 하나이며 기간은 `year`, `month`, `day`다. 이것은
  database의 파티션이 아니라 markdown corpus의 저장에 대한 선언이다. SQL backend(MySQL, PostgreSQL,
  SQLite)는 orm이 정한 대로 이 절을 무시하므로(orm `docs/dbspec.md`, `T119`) database export(spec 70)는
  샤드된 table에 table 하나를 만들고 모든 기간의 행을 table이 읽히는 순서로 insert한다. 절은 `files`와
  `document` 저장에 적용되며, orm은 `block` table, 자기 참조 외래 key가 있는 table, `order`가 있는
  table에서 거절한다. key 범위, hash, 목록에 의한 shard와 column
  둘 이상의 shard는 정의하지 않는다.

## Key

- `files` 저장 table은 단일 column primary key를 갖고, `document`나 `block` 저장 table은 composite primary key를 가질 수 있다([schema](30-schema.md)). 음수 integer key는
  거절된다: 그 `-`가 key를 시작하여 아래 모양에 어긋난다. 쓰기에서는 `CONSTRAINT`이고 읽기에서는 `format.block`이다.
  모든 저장에서 모양을 깨는 key가 그렇듯이 말이다.
- `files` 행의 key는 file 이름에서 `.flowmark.md`를 뺀 것이다.
- table 행의 key는 그 타입의 어떤 한 줄 text든 되며, cell에 필요하면 따옴표로 쓴다([canonical format](10-canonical-format.md)). 음수 정수도 그곳에서는 key다. 아래의
  형식은 따옴표 형식이 없는 `files` 행과 list 행의 key를 묶는다.
- 모든 key는 동시에 file 이름, URL path 조각, heading anchor의 일부다. key는 `A`–`Z`, `a`–`z`, `0`–`9`,
  `_`, `-`로 이루어지며 문자나 숫자로 시작한다: 선행 `_`는 강조로 렌더되어 보이는 key와 그 anchor를
  모두 바꾸기 때문이다. key의 모양은 ASCII다: 이 문자들이 key의 알파벳 전부이며 다른 문자는 key 문자가 아니다. 대소문자 비교와
  소문자 anchor가 이 글자들에 대해서만 정의되기 때문이다. key는 `-_`를 담지 않는다:
  구두점 뒤의 `_`는 left-flanking이어서 강조를 열 수 있으므로(CommonMark 0.31.2 §6.2), key `a-_b_`는 `a-`와 강조된 `b`로 렌더되어 보이는 key와 그 anchor를 모두 바꾸기 때문이다.
- key는 대소문자를 무시하고 table 안에서 유일하다. 대소문자를 구분하지 않는 file system과 소문자로 된
  anchor가 그렇지 않으면 충돌하기 때문이다. 대소문자를 무시하고 같은 table의 다른 key와 겹치는 key는 그것을 새로 만드는 쓰기에서 `DUPLICATE_KEY`이고, validator의 `key.duplicate`다. file을 읽을 수 없게 하지 않는다. 다만 같은 file 안에서 바이트 단위로 같은 key를 가진 두 행은, 둘 다 상세 섹션을 가지면 두 섹션이 같은 heading 텍스트를 가지므로([canonical format](10-canonical-format.md#detail-headings-and-anchors)), 그리고 둘이 한 block에 있을 때 상세 섹션이 그 key를 가리키거나 그 block의 `parent` cell이 그 key를 가리키면 섹션이나 부모를 배정할 수 없으므로([canonical format](10-canonical-format.md#document-order)) `format.block`이다; 서로 다른 block의 행은 block이 배정한다; 바이트 단위로 같은 key를 가진 두 행은 그 table을 사용할 수 없게 한다([canonical format](10-canonical-format.md#diagnostics)).
- 외래 key는 참조된 key와 byte 단위로 맞는다. key와 대소문자만 다른 link 텍스트는 dangling 참조다.
- 대소문자를 무시하고 `CON`, `PRN`, `AUX`, `NUL`, `COM0`–`COM9`, `LPT0`–`LPT9`와 같은 key나 table 이름은
  거절한다: table 이름은 schema 오류이고, key는 `CONSTRAINT`다. Windows의 예약된 file 이름이기 때문이다.
- `files` key는 최대 243 byte이며, 그래서 file 이름이 255 byte 안에 든다.
- key 타입의 정식 텍스트가 아닌 file 이름은 `format.block`이다: 정수 key 7의 정식 file 이름은
  `7.flowmark.md`이고 `007.flowmark.md`는 아니다.
- `identity`는 `i64` primary key column에만 허용된다([schema](30-schema.md)): key를 생략한 insert는 가장 큰 key와 표의
  counter([AUTOINCREMENT counter](40-ops.ko.md#sql-statements)) 가운데 큰 것보다 하나 큰 key를 lock 아래에서 받으므로, 같은
  key를 두 번 받지 않는다. git을 되감으면 counter도 행과 함께 되감기며, 그 뒤의 재사용은 받아들인다 — 할당의 기록은 git
  history다.
- text key는 insert가 주며, 이를 빠뜨린 insert는 `IR_INVALID`다: 흔한 database가 그렇듯 엔진은 text
  key를 할당하지 않는다(orm DBS-77은 `key_prefix`를 거둔다). 손으로 쓴 key는 유일하고 column type을
  가지면 받아들인다.
- key는 불투명하다: ID 문법은 없고 key에서 관계를 읽지 않는다.

## 제목과 body

- `title <column>`과 `body <column>`은 table마다 선택하는 setting이다. `list` 저장과 `files` 저장은
  `title`을 요구하고 — 줄 텍스트, H1 — `table` 저장은 cell로 선언할 수 있다. 선언된 제목은 non-null이며
  빈 텍스트를 거절한다.
- `body`는 `markdown` column이며 NULL이 아니다. `files` 행에서는 H1과 field list 뒤의 텍스트이고,
  상세 섹션이 있는 행에서는 상세 섹션의 첫 최상위 level ≤ h + 1 heading 앞 텍스트다([canonical format](10-canonical-format.md#sections-and-their-values)의 CommonMark block 구조로). 상세 섹션이 없는 행의 body는
  빈 텍스트다.
- 칸반, 목록, 간트 view는 entity table이 둘 다 선언할 것을 요구한다.

## 행 scope

- `files` 행의 scope는 그 file이고, 섹션은 level 2다.
- block이나 document 행의 scope는 상세 섹션 `<h> <table> row <key>`이고(level b 블록 섹션 아래에서 h = b + 1,
  document file에서 h = 2), 섹션은 level h + 1이다.
- `markdown` column은 모든 저장에서 언제나 행 scope의 섹션이다 — field나 cell이 아니다. body가 아닌
  `markdown` column(non-null이면 언제나, nullable이면 NULL이 아닐 때)과 자식 블록은 각각 column이나 자식 table
  이름을 가진 행 scope의 섹션이며, 자식 블록은 행 scope에 한 번만 서고 두 번째 것은 `format.section`이다([canonical format](10-canonical-format.md#sections-and-their-values)); 부모의
  markdown column과 같은 이름의 자식 table은 schema 오류다.

## 계층

- 계층은 self foreign key `parent`다. list에서는 들여쓰기가 parent이고 column을 쓰지 않으며, table과
  file은 column을 쓴다.
- parent를 추측하지 않고 순환은 거절한다: 쓰기에서 `CONSTRAINT`다. 읽을 때 한 file의 행 사이의 순환은 `parent.cycle`이며 그 file은 저장 순서를 갖지 못하므로 읽을 수 없다. 여러 `files` file의 행에 걸친 순환은 validator의 `parent.cycle`이며 어떤 file도 읽을 수 없게 하지 않는다. block이나 document 행의 `parent`는 그 행의 block — 행을 담은 하나의 list나 table이며, document 행에서는 그 file의 행 — 의 행을 가리킨다. 그 밖의 것은 쓰기에서 `CONSTRAINT`, 읽기에서 `format.block`이다([canonical format](10-canonical-format.md#document-order)). 근거: 부모와 자식은 배치 부모를 공유하며, 그렇지 않으면 자식이 저장 순서에서 자리를 잃고, list의 들여쓰기는 다른 block에 닿을 수 없기 때문이다.

## 순서

- `order <column>`은 default 없고 key, index, check에 들지 않는 non-null `i32`나 `i64` column을
  가리킨다(orm DBS-79). 값은 같은 배치 부모와 같은 `parent`를 가진 행, 곧 형제 사이에서 행이 서는
  자리다: 형제는 값의 오름차순으로 서고, 값을 공유하는 행은 key 순서(정수 key는 숫자, 문자는
  codepoint)로, 값이 column의 type이 아닌 행은 다른 모든 행 뒤에 key 순서로 선다.
- 이 column은 모든 저장에서 저장 scalar이며, 그 저장의 다른 scalar가 쓰이는 곳에 쓰인다: list 행의
  field, table의 cell, `files` 행의 field list 항목. 다른 non-null 저장 scalar처럼 필수이므로 이것이
  없는 행은 `format.block`이고, type이 아닌 값은 validator의 `type`이다
  ([constraints](50-constraints.ko.md#계층과-순서)). 읽기는 쓰인 값을 그대로 답하며, column에 대한 IR
  `where`나 `order`도 그 값으로 비교한다.
- 아무것도 이 값을 할당하지 않는다: insert는 default 없는 다른 non-null column처럼 이를 주며, 이를
  빠뜨린 insert는 orm이 닿는 database에서처럼 `IR_INVALID`다. type의 어떤 값이든 받아들인다. 형제가
  값을 공유해도, 빈틈이 있어도 결함이 아니다. column의 쓰기, parent 변경, delete는 쓴 행의 file이나
  그 문서만 다시 쓴다: 어떤 형제도 다시 번호 매기지 않고 빈틈도 닫지 않는다. 근거: 흔한 database는
  위치를 할당하지 않으므로 엔진은 application이 쓴 값만 가진다. 저장된 값은 다른 column의 update처럼
  이동이 한 행만 쓰게 한다.
- block과 document 저장에서 형제 묶음의 행은 그 값의 순서대로 줄에 서므로, 형제가 다른 순서로 선
  문서는 `format.noncanonical`이며, 쓰기는 그 순서로 그린다. 행을 옮기면 subtree가 함께 옮겨진다.
- `order` column이 없으면 형제는 key 순서(정수 key는 숫자, 문자는 codepoint)로 서며 각 자식은 부모를
  따른다. 다른 줄 순서는 `format.noncanonical`이다.
- **저장 순서**는 tree의 전위 순회다: 각 행, 그다음 형제 순서의 자식. 다른 배치 부모의 행은 부모의
  저장 순서를 따른다. IR `order` 없는 읽기는 저장 순서를 돌려주고, order column에 대한 IR `order`는
  그 값으로 정렬하며 동률은 저장 순서다. 저장 순서는 읽을 수 있는 모든 table에서 전순서다. `files`
  저장에서 `parent`가 NULL이거나 그 table의 어떤 행도 가리키지 않는 행은 root다. root는 형제 묶음별로
  선다 — `parent`가 NULL인 묶음이 먼저, 그다음 `parent`가 어떤 행도 가리키지 않는 각 묶음이 그
  `parent` 값의 key 순서로 — 각 묶음은 형제 순서로, 각 root 뒤에는 그 subtree가 전위 순회로 온다.
  어떤 root도 닿지 않는 행 — `parent`가 그 column의 type이 아닌 행, 여러 file에 걸친 parent 순환 위의
  행, 그리고 그 아래의 모든 행 — 은 닿은 모든 행 뒤에 corpus 순서로 온다. `order` column이 있는 형제
  사이에서 값을 공유하는 행은 key 순서로 서며, 값이 그 type이 아닌 행은 다른 행 뒤에 key 순서로
  선다. 근거: 읽기는 다른 행의 validator 진단 때문에 실패하지 않으므로, 진단을 가진 corpus의
  저장 순서는 corpus 순서처럼 corpus만의 함수여야 한다.

## 관계

- foreign key는 delete에서 `restrict`, `cascade`, `set_null` action을 가지며, 엔진이 쓰기 안에서
  적용한다([constraints](50-constraints.md)).
- 외래 key 값은 link다. link의 텍스트는 참조된 key이고, target은 참조된 행에서 유도된다. 정식 형식은
  [canonical format](10-canonical-format.md)이 정한다. block 행에서 그 행을 담은 `files` 행으로 가는 link는 그 file의 이름이며, 그 file 자신의 key에서 유도된다([canonical format](10-canonical-format.md#foreign-keys-as-links)). 참조된 key가 없는 dangling 외래 key는 validator의 `reference.dangling`이며, 그것을 새로 만드는 쓰기는 `FOREIGN_KEY`다. file을 읽을 수 없게 하지 않는다. 다만 block이나 document 행의 `parent`가 그 block의 어떤 행도 가리키지 않으면 `format.block`이다([canonical format](10-canonical-format.md#foreign-keys-as-links)). 모든 관계는 `#<anchor>`가 GitHub, GitLab, VS Code가 그 heading에 대해 만드는 anchor인 link다([canonical format](10-canonical-format.md#detail-headings-and-anchors)가 정한다). Pandoc은 주장 대상이 아니다: 기본 설정은 상대 경로 link target을 다시 쓰지 않으므로, corpus file 사이의 link가 그 출력에서 따라가진다고 주장하지 않는다.
- file 간 one-to-many는 자식 table을 `store files`로 두고 foreign key를 그 field list에 쓴다. file 안의
  one-to-many는 자식 table을 `store block`으로 둔다.
- 다대다는 `identity` key를 가진 link table이다.
- 부모 file 안의 자식 행은 자기 file이 없고 부모의 transaction이 덮는다.
