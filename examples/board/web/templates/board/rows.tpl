<div class="board-tools">
<span>정렬</span>
<a class="chip{? sort == ''} current{/}" href="/board?sort=">최신순</a>
<a class="chip{? sort == 'title'} current{/}" href="/board?sort=title">제목순</a>
<a class="chip{? sort == 'author'} current{/}" href="/board?sort=author">작성자순</a>
<button type="button" class="chip{? compact} current{/}" hy-set="compact={? compact}false{:}true{/}">좁게 보기</button>
</div>
{= listHtml | raw}
