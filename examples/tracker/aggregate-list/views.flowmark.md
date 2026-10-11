# views
| id | title | kind | entity | filter | sort | group_by |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 작업 보드 | kanban | items | {"where":{"items":[]},"params":[]} | [] | state |
| 2 | 담당자별 목록 | list | items | {"where":{"items":[{"pred":{"column":"assignee","op":"is_not_null"}}]},"params":[]} | [{"column":"title"}] | assignee |
| 3 | 일정 | gantt | items | {"where":{"items":[]},"params":[]} | [{"column":"due_on"}] |  |

## views row 1

모든 item을 상태별 column으로 본다.

## views row 2

담당자가 있는 item을 담당자별로 묶는다.
