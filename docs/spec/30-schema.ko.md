<!-- doc-id: spec-schema -->
<!-- source-sha256: 6c88d8ca124e64a1716291620eb2672d7b90f7b6593bfb40c965d82d387b8483 -->
# Schema

[English](30-schema.md)

이 명세는 schema 언어 관계를 확정한다: dbspec이 root grammar이자 authority이고, 문서는 그
projection과 임시 fragment만 운반한다.

## 파생 판정

- **dbspec ⇒ frontmatter.** corpus의 `.dbs` dbspec이 유일한 authority다. frontmatter DDL은 독립
  언어가 아니다 — frontmatter에 embed된 dbspec fragment이며, 파생은 항상 dbspec에서 문서로
  흐른다.
- 근거: schema는 law이고 row는 data다 — law의 문서별 사본이 어긋나는 것 자체가 drift bug
  class다; orm의 planner·IR·conformance는 dbspec manifest hash에 binding된다; 반대 방향은 corpus
  schema를 문서별 선언의 merge로 구성해 branch 간 DDL reconciliation을 만든다.

## Embedding

- fragment는 항상 **YAML literal block scalar**(`|`)다 — 이것이 `yes`·`on` 같은 이름, datetime의
  colon, fragment 안의 `#` 같은 YAML-적대적 token을 해결하고 parser와 type system을 하나로
  유지한다.
- dbspec diagnostic은 **offset-mapping**되어 file 좌표를 낸다 — embed된 fragment의 error가
  문서 안의 제자리를 가리킨다.
- fragment는 embed 시점에 canonicalize하고(LF, 마지막 개행), stamp는 **canonical** fragment를
  담는다 — 두 번째 minified 출력 형식은 없다.

## Standalone 문서

- repo 밖의 문서는 자기 자신의 **임시 authority**다: embed된 fragment가 하나의 문서 집합 위에서
  같은 IR로 compile되고, standalone fragment에 `use` 선언은 유효하지 않다.
- corpus로 import되면 repo의 `.dbs`가 authority가 되고 embed된 fragment는 정합성 검사 대상이
  된다: 불일치는 `schema.drift`다.

## Schema stamp

corpus의 모든 문서는 `.dbs`에서 render된 stamp를 운반한다: manifest hash, `schema_version`, 자기
table의 canonical fragment. stamp는 **data가 아니라 verified cache**다 — repo manifest와의 불일치는
located diagnostic이지 silent reparse가 아니며, 사람이 편집할 수 없다.

## Markdown 고유 schema 표면

- **manifest-level setting만 있고 render되는 type은 없다.** markdown 성질의 column trait와
  checklist child-table sugar는 `updated`·`soft_delete`처럼 manifest의 setting이다: 실제
  database는 이들을 평범한 `text`로 introspect하므로 render→introspect→같은 문서가 성립한다.
  state machine도 같은 방식의 manifest-level setting이며 자체 sub-grammar를 가지고 — SQL
  backend는 `navigation`처럼 무시하고 markdown validator가 강제한다.
- **`identity` column은 markdown corpus에서 거부된다**: key는 [ID grammar](10-canonical-format.ko.md)로
  사람이 부여하고, primary key를 생략한 insert는 error이며 `flowmark next-id`가 할당 보조다.
  git 되감기에서의 key 비재사용은 설계상 포기한다 — 할당 기록은 git history다.
- **foreign-key action은 restrict뿐**이다; `cascade`와 `set_null`은 schema parse에서 거부된다 —
  executor를 우회하는 writer에게 구현 불가능하므로.
- **`audit`와 `immutable`은 advisory다**: 위반은 validate 시점 거부이고 사람의 파일 편집에 대한
  trigger 시점 예방은 없다.
- **기본값**: non-null 기본값 column이 빠진 사람 작성 파일은 validate가 거부하고 `flowmark
  ingest`가 기본값을 명시적으로 찍는다.
