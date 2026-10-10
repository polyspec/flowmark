<!-- doc-id: spec-migration -->
<!-- source-sha256: 3aab0a0164679c39240e8512e9cf9189bb6131d0f2d49f824bfc2588ba47b8be -->
# Migration (범위를 정한 뼈대)

[English](80-migration.md)

이 문서는 schema 진화의 범위와 확정된 판정을 적는다. 세부 spec은 v1 범위를 넘는 진화가 시작될 때 쓴다.

## 범위

- **v1은 corpus 생성과 추가적 column 추가를 지원한다.** `flowmark install`이 catalog에서 저장 뼈대를
  만든다([schema](30-schema.md)). dbplan chain(orm `plans.md` lifecycle)은 `markdown://`에서
  `CAPABILITY_UNSUPPORTED`다: 그 server session, hold table, introspection 장치는 v1에서 markdown 대응이
  없다.
- 추가된 nullable column은 `files` 행의 field list, list 행, 섹션을 다시 쓸 필요가 없다. 없는 항목, field, 섹션은 NULL이다. `table` 저장은 각 행에 column의 header와 빈 cell을 얻으며, 같은 일회성 migration transaction이 쓰고, 그 대상은 없는 header의 `format.block`에 대해 면제된다. 추가된 non-null column은 default와 함께 추가하며, 일회성 migration transaction이 기존의 모든 행에 default를 쓴다(모든 non-null column은 쓰므로([canonical format](10-canonical-format.md))). 그 대상은 추가된 column이 직접 일으키는 진단 — 없는 값의 `format.block`과, column이 추가하는 상세 섹션으로의 link의 `format.noncanonical` — 에 대해 commit이 거절되지 않으며, 대상 안의 다른 `format.*` 진단은 file을 밝히며 거절한다. 추가된 column이 일으키는 진단은 그 migration에 대해 table을 사용할 수 없게 하지 않는다.

## 확정된 판정

- **추가 우선**: 이름 변경은 명시적인 단계이며 "새로 추가하고 옛것을 지우기"가 아니다.
- **저장 변경은 transaction이다**: table을 한 저장에서 다른 저장으로 옮기면 모든 행을 transaction 하나로
  다시 쓰며, 행은 전후로 같게 읽힌다; `format.*` 진단이 있는 file은 거절하며 file을 밝힌다.
- **key는 불투명하다.** 그래서 prose의 참조를 고쳐 쓰는 key codemod는 없다. key 변경은 어떤 행이든 그 key를
  참조하는 동안 거절한다. 참조하는 행들은 먼저, 같은 transaction에서 호출자가 갱신한다.

## 나중을 위해 열어 둔 것

table 전체 이동을 넘는 저장 선언의 진화, file 단위 단계의 효과 어휘, markdown plan dialect의 거절이나
설계.
