<!-- doc-id: spec-document-model -->
<!-- source-sha256: 45685199034e440f5728633facaee4acf3d59f4c1a8f6923ea798a04c5f26604 -->
# 문서 모델

[English](20-document-model.md)

이 명세는 corpus directory가 table과 row가 되는 방식을 확정한다. mapping은 전체적이다: 문서의
모든 byte는 정확히 하나의 모델링된 곳에 속하거나, parser가 그것을 보고한다.

## Corpus와 table

- **Folder = table.** dbspec table의 `table` 선언이 연결하는 directory. walk는 `.flowmark/`와
  숨김 항목을 건너뛴다; corpus 안의 symlink와 hardlink는 diagnostic이고 nested corpus root는
  error다.
- **File = row.** markdown 문서 하나가 row 하나; primary key는 table이 선언한 key column으로
  header comment의 `doc-id`가 운반하고, task-key table에서는 [canonical format](10-canonical-format.ko.md)의
  item grammar를 따른다.
- **번역 쌍은 하나의 row다.** 영어 문서가 row이고 Korean twin은 그 row의 projection이다 — twin은
  같은 `doc-id`를 가지고 영어 파일 raw bytes의 `source-sha256`을 보유한다. twin은 결코 두 번째
  row가 아니고 자기 column data도 갖지 않는다.

## Column

- **Frontmatter = typed column.** frontmatter block의 모든 key는 table의 column이다; 값은
  [frontmatter profile](10-canonical-format.ko.md)을 따른다.
- **Preamble = 예약된 long-text column `preamble`.**
- **H2 section = 선언된 long-text column.** 선언된 column마다 heading 하나; 선언되지 않은 heading은
  diagnostic; 없는 section은 NULL이고 빈 section은 빈 text다.
- **모델링된 곳 밖의 body byte는 보존 payload다** — renderer의 보존 boundary가 정하는 대로 row에
  속하고, query는 row의 보존된 내용 이외로는 들여다보지 않는다.

## Child table

- **구조화 block은 child table이다.** 문서 안의 checklist block, status table, markdown table은
  소유 문서를 향한 foreign key를 가진 선언된 child table의 row로 mapping되고, 배치는 schema가
  child table마다 선언한다.
- **child row는 부모 문서 안에 존재한다** — 자기 파일이 없고 부모의 transaction에 포함된다. 참조
  action은 restrict뿐이다 ([제약](50-constraints.ko.md)).
- **markdown-table cell은 줄바꿈을 가질 수 있는 값을 담을 수 없다**; schema는 그런 column을
  checklist나 section 배치에 둔다.

## Aggregate와 분할

세 shape가 checklist와 그 내용이 파일에 놓이는 방식을 포괄한다. 모두 같은 primitive — file
as row, child block, foreign-key column — 로 구성된다:

- **단일 파일 (aggregate).** checklist와 내용과 checkbox가 하나의 문서다: item이 그 안의
  child row다.
- **1:n (분할).** 부모 문서가 제목·index·checkbox를 가지고, 각 상세는 자기 table의 문서로서
  foreign-key column(item ID 또는 doc-id)으로 부모 항목에 묶인다. 각 상세는 자기 table의
  schema를 따른다.
- **1:n:1 (두 홉).** 중간 문서가 foreign-key column으로 정확히 하나의 부모 항목에 묶이고 그
  자신이 child checklist block을 가진다.

foreign-key column은 restrict뿐이다: 없는 부모나 없는 상세는 located dangling finding이지
cascade가 아니다.

## Binding

frontmatter는 row의 data와 binding을 함께 운반한다: `table`은 소유 table을, `schema_version`은
문서가 쓰인 schema generation을 찍고, standalone 문서는 추가로 자기 [schema fragment](30-schema.ko.md)를
embed한다. binding 불일치는 `schema.drift` diagnostic이지 silent reparse가 아니다.
