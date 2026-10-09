<!-- doc-id: spec-migration -->
<!-- source-sha256: 6d93a81955da9e216ba2dc4fd3dd2712d1daad2fcb5461d0db8ed2075220dad9 -->
# Migration (범위 skeleton)

[English](80-migration.md)

이 문서는 schema 진화의 범위와 확정된 판정을 서술한다. v1 범위를 넘는 schema 진화가 시작될 때
상세 명세를 작성한다.

## 범위

- **v1은 corpus 생성, additive column 추가, 명시적 transactional codemod를 지원한다.** dbplan
  chain (orm `plans.md` lifecycle)은 markdown corpus에서 거부한다: server session, hold table,
  introspection 장치에 v1 markdown 대응이 없고, design 단계는 additive-only 진화로 충분하다.
- codemod는 transaction이다: `flowmark renumber`는 하나의 item과 모든 구조적 참조를 하나의
  transaction에서 다시 쓴다 — suffix는 base와 함께 변경된다 — 그리고 dangling 참조나 key 충돌이
  생기면 transaction을 거부한다.

## 확정된 판정

- **additive-first**: 새 column은 migration이 값을 찍기 전까지 optional이다. rename은 명시적
  step이고 "새로 추가하고 오래된 것 제거"로 대체하지 않는다.
- **참조 재작성이 payload 보존의 명시적 예외다** (canonical format): codemod는 산문의 참조
  token을 다시 쓰고 fenced block과 inline code span 안은 다시 쓰지 않는다.
- **checklist cutover** — checklist application이 자기 corpus를 읽는 시점 — 는 하나의
  transaction이다: tracker가 `config/checklist.json`에서 제거되고, 문서가 corpus 형식으로
  변환되고, 같은 commit에서 `flowmark validate`가 통과한다.

## 이후에 확정할 것

markdown plan dialect (또는 영구 거부), file 단계의 effect 어휘, 범위 renumber의 충돌 처리.
