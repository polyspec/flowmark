<section class="board-form">
<h1>글쓰기</h1>
<form method="post" action="/board/create">
<input type="hidden" name="_csrf" value="{= csrf}">
<label>제목 <input name="title" value="{= values.title}" maxlength="100"></label>
{? errors.title}<p class="error">{= errors.title}</p>{/}
<label>작성자 <input name="author" value="{= values.author}" maxlength="30"></label>
{? errors.author}<p class="error">{= errors.author}</p>{/}
<label>내용 <textarea name="body" rows="12">{= values.body}</textarea></label>
{? errors.body}<p class="error">{= errors.body}</p>{/}
<div class="form-actions">
<a href="/board" class="button secondary">취소</a>
<button type="submit">등록</button>
</div>
</form>
</section>
