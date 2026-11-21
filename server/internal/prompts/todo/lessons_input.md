{{if eq .Language "日本語"}}タスク：{{.Title}}
{{with .Request}}
ユーザーの最初の依頼：
{{.}}
{{end}}{{with .Goal}}
目標：{{.}}
{{end}}{{if .Criteria}}
承認の基準：
{{range .Criteria}}- {{.}}
{{end}}{{end}}{{if .Changes}}
途中で変わった範囲：
{{range .Changes}}- {{.Change}}{{with .Why}}（理由：{{.}}）{{end}}
{{end}}{{end}}{{if .Reviews}}
レビュー（古い順）：
{{range .Reviews}}- {{.Verdict}}{{if .Round}} · {{.Round}} 回目{{end}}{{with .By}} · {{.}}{{end}}{{with .Note}}：{{.}}{{end}}
{{range .Unmet}}  - 未達：{{.Criterion}}{{with .Evidence}}（{{.}}）{{end}}
{{end}}{{end}}{{end}}{{if .Answers}}
ユーザーが途中で答えた質問：
{{range .Answers}}- {{.Question}} → {{.Choice}}
{{end}}{{end}}{{if .Result}}
終了時の各基準の結果：
{{range .Result}}- {{.Criterion}}：{{.State}}{{with .Evidence}}（{{.}}）{{end}}
{{end}}{{end}}{{if .ExpectMinutes}}
見積もり {{.ExpectMinutes}} 分{{if .ActualMinutes}}、実際 {{.ActualMinutes}} 分{{end}}
{{end}}{{with .Reply}}
Agent の最後の返信：
{{.}}
{{end}}{{if .Known}}
このプロジェクトに記録済みの教訓：
{{range .Known}}- [{{.ID}}] {{.Kind}}：{{.Text}}
{{end}}{{end}}{{if .Deleted}}
ユーザーが削除した教訓（二度と学ばない）：
{{range .Deleted}}- [{{.ID}}] {{.Kind}}：{{.Text}}
{{end}}{{end}}{{else if .Language}}Task: {{.Title}}
{{with .Request}}
The user's original request:
{{.}}
{{end}}{{with .Goal}}
Goal: {{.}}
{{end}}{{if .Criteria}}
Acceptance criteria:
{{range .Criteria}}- {{.}}
{{end}}{{end}}{{if .Changes}}
Scope changes along the way:
{{range .Changes}}- {{.Change}}{{with .Why}} (why: {{.}}){{end}}
{{end}}{{end}}{{if .Reviews}}
Reviews (oldest first):
{{range .Reviews}}- {{.Verdict}}{{if .Round}} · round {{.Round}}{{end}}{{with .By}} · {{.}}{{end}}{{with .Note}}: {{.}}{{end}}
{{range .Unmet}}  - not met: {{.Criterion}}{{with .Evidence}} ({{.}}){{end}}
{{end}}{{end}}{{end}}{{if .Answers}}
Questions the user answered while it ran:
{{range .Answers}}- {{.Question}} → {{.Choice}}
{{end}}{{end}}{{if .Result}}
Each criterion at the end:
{{range .Result}}- {{.Criterion}}: {{.State}}{{with .Evidence}} ({{.}}){{end}}
{{end}}{{end}}{{if .ExpectMinutes}}
Estimated {{.ExpectMinutes}} minutes{{if .ActualMinutes}}, took {{.ActualMinutes}}{{end}}
{{end}}{{with .Reply}}
The Agent's last reply:
{{.}}
{{end}}{{if .Known}}
Lessons this project already has:
{{range .Known}}- [{{.ID}}] {{.Kind}}: {{.Text}}
{{end}}{{end}}{{if .Deleted}}
Lessons the user deleted (never learn them again):
{{range .Deleted}}- [{{.ID}}] {{.Kind}}: {{.Text}}
{{end}}{{end}}{{else}}任务：{{.Title}}
{{with .Request}}
用户最初的要求：
{{.}}
{{end}}{{with .Goal}}
目标：{{.}}
{{end}}{{if .Criteria}}
验收标准：
{{range .Criteria}}- {{.}}
{{end}}{{end}}{{if .Changes}}
做的过程中改过的范围：
{{range .Changes}}- {{.Change}}{{with .Why}}（原因：{{.}}）{{end}}
{{end}}{{end}}{{if .Reviews}}
验收记录（先到后）：
{{range .Reviews}}- {{.Verdict}}{{if .Round}} · 第 {{.Round}} 轮{{end}}{{with .By}} · {{.}}{{end}}{{with .Note}}：{{.}}{{end}}
{{range .Unmet}}  - 没达到：{{.Criterion}}{{with .Evidence}}（{{.}}）{{end}}
{{end}}{{end}}{{end}}{{if .Answers}}
用户在做的过程中回答的问题：
{{range .Answers}}- {{.Question}} → {{.Choice}}
{{end}}{{end}}{{if .Result}}
结束时每条标准的结果：
{{range .Result}}- {{.Criterion}}：{{.State}}{{with .Evidence}}（{{.}}）{{end}}
{{end}}{{end}}{{if .ExpectMinutes}}
预计 {{.ExpectMinutes}} 分钟{{if .ActualMinutes}}，实际 {{.ActualMinutes}} 分钟{{end}}
{{end}}{{with .Reply}}
Agent 最后的回复：
{{.}}
{{end}}{{if .Known}}
这个项目已经记下的经验：
{{range .Known}}- [{{.ID}}] {{.Kind}}：{{.Text}}
{{end}}{{end}}{{if .Deleted}}
用户删掉的经验（不要再学）：
{{range .Deleted}}- [{{.ID}}] {{.Kind}}：{{.Text}}
{{end}}{{end}}{{end}}
