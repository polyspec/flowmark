<!-- doc-id: spec-migration -->
<!-- source-sha256: fea7b8057777b1068028bc939a33e07c3a2359cac10381fc2484a75f29086277 -->
# Migration (범위를 정한 뼈대)

[English](80-migration.md)

이 문서는 schema 진화의 범위와 확정된 판정을 적는다. 세부 spec은 v1 범위를 넘는 진화가 시작될 때 쓴다.

## 범위

- **v1은 corpus 생성과 추가적 column 추가를 지원한다.** `flowmark install`이 catalog에서 저장 뼈대를
  만든다([schema](30-schema.md)). dbplan chain(orm `plans.md` lifecycle)은 `markdown://`에서
  `CAPABILITY_UNSUPPORTED`다: 그 server session, hold table, introspection 장치는 v1에서 markdown 대응이
  없다.
- 추가된 nullable column은 다시 쓸 필요가 없다: 없는 field, cell, key, 섹션은 NULL이다. 추가된 non-null
  column은 default와 함께 추가하며, 모든 non-null column은 쓰므로([canonical format](10-canonical-format.md))
  일회성 transaction이 기존의 모든 행에 default를 쓴다.

## 확정된 판정

- **추가 우선**: 이름 변경은 명시적인 단계이며 "새로 추가하고 옛것을 지우기"가 아니다.
- **저장 변경은 transaction이다**: table을 한 저장에서 다른 저장으로 옮기면 모든 행을 transaction 하나로
  다시 쓰며, 행은 전후로 같게 읽힌다.
- **key는 불투명하다.** 그래서 prose의 참조를 고쳐 쓰는 key codemod는 없다. key 변경은 key와 그것을
  참조하는 모든 foreign key의 update이며, restrict-only 참조가 dangling이 되는 동안은 거절한다.

## 나중을 위해 열어 둔 것

table 전체 이동을 넘는 저장 선언의 진화, file 단위 단계의 효과 어휘, markdown plan dialect의 거절이나
설계.
