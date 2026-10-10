<!-- doc-id: spec-document-model -->
<!-- source-sha256: 1c1b4d5ed45bcbfb74a866d4baeb650c6aea166524623d155e690afbabec2f0b -->
# 문서 모델

[English](20-document-model.md)

이 spec은 corpus가 table과 행이 되는 방식을 고정한다. 대응은 전체적이다: 문서의 모든 byte는 모델링된
자리 하나에 속하거나 parser가 보고한다. byte 형식은 [canonical format](10-canonical-format.md)에,
선언은 [schema](30-schema.md)에 있다.

## Corpus root

- corpus root는 command나 server에 준 directory다. `dbspec/*.dbs` — catalog — 와 `.flowmark/`를 담는다.
  위로 거슬러 찾지 않는다.
- walk는 `.flowmark/`, `dbspec/`, 숨은 항목을 건너뛴다. corpus 안의 symlink나 hard link는 진단이고,
  중첩된 corpus root는 오류다.
- 이 repository는 corpus가 아니다: 자기 checklist는 kit이 추적하는 문서다.

## 저장

각 table은 정확히 하나의 저장을 선언한다. 없는 table은 schema 오류이며 기본값을 추측하지 않는다.

- **`store files`** — corpus root 아래 table 이름의 폴더에 행마다 file 하나. insert는
  `<table>/<key>.md`를 쓴다. 엔진은 file 이름을 바꾸지 않으며 key가 바뀌어도 file은 그대로다.
- **`store document list`, `store document table`** — 모든 행이 corpus root의 file `<table>.md` 하나에
  있다.
- **`store block <fk> list`, `store block <fk> table`** — 이름 붙은 foreign key가 가리키는 부모 행의
  scope 안의 행.

aggregate냐 split이냐는 저장 선언이다. 논리 관계는 모든 저장에서 같으므로 같은 행을 어느 쪽으로든
저장할 수 있다.

## Key

- markdown table은 단일 column primary key를 갖는다. composite primary key는 schema 오류다.
- `files` 행의 key는 `<!-- doc-id: -->` 값이다. file 이름은 의미가 없다. doc-id는 corpus의 행과
  `document` file 사이에서 유일하며 twin은 같은 doc-id를 공유한다.
- `identity`는 모든 markdown table에 허용된다: key를 생략한 insert는 lock 아래 할당된 `max+1`을 받는다.
  git을 되감은 뒤의 재사용은 받아들인다 — 할당의 기록은 sequence가 아니라 git history다.
- `key_prefix <p>`를 가진 `varchar` key는 `<p><n + 1>`로 할당되며, n은 `<p><n>` 꼴 key 가운데 선행
  0 없는 가장 큰 10진 접미사다. 둘 다 없는 key는 insert가 주어야 한다. 손으로 쓴 key는 유일하고 column
  type을 가지면 받아들이며, `<p><n>` 꼴이 아닌 key는 n에 세지 않는다.
- key는 불투명하다: ID 문법은 없고 key에서 관계를 읽지 않는다.

## 제목과 body

- `title <column>`과 `body <column>`은 table마다 선택하는 setting이다. `list` 저장과 `files` 저장은
  `title`을 요구하고 — 줄 텍스트, H1 — `table` 저장은 cell로 선언할 수 있다. 선언된 제목은 non-null이며
  빈 텍스트를 거절한다.
- `body`는 `markdown` column이며 NULL이 아니다. file의 H1 뒤 텍스트, 또는 행 상세 섹션의 첫 하위
  heading 앞 텍스트다. 상세 섹션이 없는 행의 body는 빈 텍스트다.
- 칸반, 목록, 간트 view는 entity table이 둘 다 선언할 것을 요구한다.

## 행 scope

- `files` 행의 scope는 그 file이고, 섹션은 level 2다.
- block이나 document 행의 scope는 상세 섹션 `<h> <key>`이고(level b 블록 섹션 아래에서 h = b + 1,
  document file에서 h = 2), 섹션은 level h + 1이다.
- `markdown` column은 모든 저장에서 언제나 행 scope의 섹션이다 — field나 cell이 아니다. nullable
  `markdown` column과 자식 블록은 각각 column이나 자식 table 이름을 가진 행 scope의 섹션이며, 부모의
  markdown column과 같은 이름의 자식 table은 schema 오류다.

## 계층

- 계층은 self foreign key `parent`다. list에서는 들여쓰기가 parent이고 column을 쓰지 않으며, table과
  file은 column을 쓴다.
- parent를 추측하지 않고 순환은 거절하며, 행의 블록 밖 `parent`는 쓰기에서 `CONSTRAINT`, 읽기에서
  `format.block`이다.

## 순서

- `order <column>`은 default 없고 key, index, check에 들지 않는 non-null `i32`나 `i64` column을
  가리킨다. 값은 같은 배치 부모와 같은 `parent`를 가진 행 사이에서 1부터 센 위치다.
- block과 document 저장에서는 쓰지 않는다: 읽을 때 줄 순서에서 유도되고, 쓰면 `format.block`이다.
  `files` 저장에서는 frontmatter field이며 형제 값은 정확히 1..n이다. 빈틈이나 중복은 validate 진단이다.
- insert는 이를 생략할 수 있고 행은 뒤에 붙는다. k를 주면 k에 놓는다. k로의 update는 행을 옮기고
  형제가 같은 transaction에서 따라간다. k < 1이나 k > n + 1은 `CONSTRAINT`이며 잘라 맞추지 않는다.
  `order`를 할당하는 update는 정확히 한 행과 맞아야 하며 아니면 `CONSTRAINT`다. `order` 없이
  `parent`를 바꾸면 새 형제 뒤에 붙고 옛 형제는 빈틈을 닫는다. 행을 옮기면 subtree가 함께 옮겨진다.
- `order` column이 없으면 형제는 key 순서(정수 key는 숫자, 문자는 codepoint)로 서며 각 자식은 부모를
  따른다. 다른 줄 순서는 `format.noncanonical`이다.
- **저장 순서**는 tree의 전위 순회다: 각 행, 그다음 형제 순서의 자식. 다른 배치 부모의 행은 부모의
  저장 순서를 따른다. IR `order` 없는 읽기는 저장 순서를 돌려주고, order column에 대한 IR `order`는
  그 값으로 정렬하며 동률은 저장 순서다.

## 관계

- 모든 foreign key는 restrict-only다([constraints](50-constraints.md)). file 간 one-to-many는 자식
  table을 `store files`로 두고 foreign key를 그 frontmatter에 쓴다. file 안의 one-to-many는 `store
  block`으로 둔다.
- 다대다는 `identity` key를 가진 link table이다.
- 부모 file 안의 자식 행은 자기 file이 없고 부모의 transaction이 덮는다.

## 번역

- twin은 doc-id가 어떤 행의 doc-id와 같은 file `<name>.<lang>.md`다. 행이 아니며 엔진은 쓰지 않는다.
  언어 목록은 `dbspec/flowmark.json`의 corpus setting `translations`이고 kit의 `config/documents.json`이
  이를 반영한다. 원본이 바뀐 행은 kit의 `source-sha256` 규칙에 따라 twin을 낡게 만든다.
