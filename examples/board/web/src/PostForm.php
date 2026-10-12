<?php

declare(strict_types=1);

namespace Flowmark\Board;

use Polyspec\Crudui\Form;
use Polyspec\Crudui\Generator;
use Polyspec\Crudui\Validator;
use stdClass;

/**
 * The post form and the post list, rendered and validated by crudui (spec: the form declares each field with its rule
 * and its Korean message, the list declares the columns the board shows). The compiled form template is cached for the
 * process; an instance is created per request.
 */
final class PostForm
{
    private static ?stdClass $template = null;

    private const FORM = <<<'JSON'
    {
      "type": "group",
      "properties": {
        "title": {
          "type": "text", "label": "제목", "attrs": {"maxlength": 100},
          "validate": {"required": true, "maxlength": 100},
          "messages": {"required": "제목을 입력하세요.", "maxlength": "제목은 100자 이하로 입력하세요."}
        },
        "author": {
          "type": "text", "label": "작성자", "attrs": {"maxlength": 30},
          "validate": {"required": true, "maxlength": 30},
          "messages": {"required": "작성자를 입력하세요.", "maxlength": "작성자는 30자 이하로 입력하세요."}
        },
        "body": {
          "type": "textarea", "label": "내용", "attrs": {"rows": 12},
          "validate": {"required": true},
          "messages": {"required": "내용을 입력하세요."}
        }
      }
    }
    JSON;

    private const LIST = <<<'JSON'
    {
      "columns": {
        "id": {"field": "id", "label": "번호", "format": "text"},
        "title": {"field": "title", "label": "제목", "format": {"type": "link", "href": "/board/{=id}"}},
        "author": {"field": "author", "label": "작성자", "format": "text"},
        "created": {"field": "created", "label": "작성일", "format": "datetime"}
      }
    }
    JSON;

    /** @return array{valid: bool, errors: list<array{path: string, message: string}>} the judgement of the submitted values */
    public static function validate(array $submitted): array
    {
        $result = Validator::validate(json_decode(self::FORM, false, 512, JSON_THROW_ON_ERROR), $submitted);
        return ['valid' => $result->valid, 'errors' => $result->errors];
    }

    /** Renders the create form: the fields with the values they hold, their errors under them, and the CSRF token hidden. */
    public static function render(array $values, array $errors, string $csrf): string
    {
        $form = new Form(self::template(), $values, ['language' => 'ko']);
        return Generator::renderForm($form, [
            'language' => 'ko',
            'action' => ['method' => 'post', 'url' => '/board/create'],
            'hidden' => ['_csrf' => $csrf],
            'errors' => $errors,
        ]);
    }

    /** Renders one page of posts as the board list. */
    public static function renderList(array $posts): string
    {
        $rows = array_map(static fn (array $post): array => [
            'id' => $post['id'],
            'title' => $post['title'],
            'author' => $post['author'],
            'created' => date('Y-m-d H:i', $post['created_at']),
        ], $posts);
        return Generator::renderList(json_decode(self::LIST, false, 512, JSON_THROW_ON_ERROR), $rows, ['language' => 'ko']);
    }

    private static function template(): stdClass
    {
        if (self::$template === null) {
            self::$template = Generator::compileForm(json_decode(self::FORM, false, 512, JSON_THROW_ON_ERROR), ['keyPrefix' => 'form']);
        }
        return self::$template;
    }
}
