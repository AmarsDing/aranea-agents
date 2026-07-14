## 技术交付物
### WordPress：自定义主题结构

```
my-theme/
├── style.css              # 仅主题头——不含样式
├── functions.php          # Enqueue 脚本，注册功能
├── index.php
├── header.php / footer.php
├── page.php / single.php / archive.php
├── template-parts/        # 可复用 partial
│   ├── content-card.php
│   └── hero.php
├── inc/
│   ├── custom-post-types.php
│   ├── taxonomies.php
│   ├── acf-fields.php     # ACF 字段组注册（JSON 同步）
│   └── enqueue.php
├── assets/
│   ├── css/
│   ├── js/
│   └── images/
└── acf-json/              # ACF 字段组同步目录
```

### WordPress：自定义插件样板

```php
<?php
/**
 * Plugin Name: My Agency Plugin
 * Description: Custom functionality for [Client].
 * Version: 1.0.0
 * Requires at least: 6.0
 * Requires PHP: 8.1
 */

if ( ! defined( 'ABSPATH' ) ) {
    exit;
}

define( 'MY_PLUGIN_VERSION', '1.0.0' );
define( 'MY_PLUGIN_PATH', plugin_dir_path( __FILE__ ) );

// 自动加载类
spl_autoload_register( function ( $class ) {
    $prefix = 'MyPlugin\\';
    $base_dir = MY_PLUGIN_PATH . 'src/';
    if ( strncmp( $prefix, $class, strlen( $prefix ) ) !== 0 ) return;
    $file = $base_dir . str_replace( '\\', '/', substr( $class, strlen( $prefix ) ) ) . '.php';
    if ( file_exists( $file ) ) require $file;
} );

add_action( 'plugins_loaded', [ new MyPlugin\Core\Bootstrap(), 'init' ] );
```

### WordPress：注册自定义文章类型（代码，非 UI）

```php
add_action( 'init', function () {
    register_post_type( 'case_study', [
        'labels'       => [
            'name'          => 'Case Studies',
            'singular_name' => 'Case Study',
        ],
        'public'        => true,
        'has_archive'   => true,
        'show_in_rest'  => true,   // Gutenberg + REST API 支持
        'menu_icon'     => 'dashicons-portfolio',
        'supports'      => [ 'title', 'editor', 'thumbnail', 'excerpt', 'custom-fields' ],
        'rewrite'       => [ 'slug' => 'case-studies' ],
    ] );
} );
```

### Drupal：自定义模块结构

```
my_module/
├── my_module.info.yml
├── my_module.module
├── my_module.routing.yml
├── my_module.services.yml
├── my_module.permissions.yml
├── my_module.links.menu.yml
├── config/
│   └── install/
│       └── my_module.settings.yml
└── src/
    ├── Controller/
    │   └── MyController.php
    ├── Form/
    │   └── SettingsForm.php
    ├── Plugin/
    │   └── Block/
    │       └── MyBlock.php
    └── EventSubscriber/
        └── MySubscriber.php
```

### Drupal：模块 info.yml

```yaml
name: My Module
type: module
description: 'Custom functionality for [Client].'
core_version_requirement: ^10 || ^11
package: Custom
dependencies:
  - drupal:node
  - drupal:views
```

### Drupal：实现 Hook

```php
<?php
// my_module.module

use Drupal\Core\Entity\EntityInterface;
use Drupal\Core\Session\AccountInterface;
use Drupal\Core\Access\AccessResult;

/**
 * Implements hook_node_access().
 */
function my_module_node_access(EntityInterface $node, $op, AccountInterface $account) {
  if ($node->bundle() === 'case_study' && $op === 'view') {
    return $account->hasPermission('view case studies')
      ? AccessResult::allowed()->cachePerPermissions()
      : AccessResult::forbidden()->cachePerPermissions();
  }
  return AccessResult::neutral();
}
```

### Drupal：自定义 Block 插件

```php
<?php
namespace Drupal\my_module\Plugin\Block;

use Drupal\Core\Block\BlockBase;
use Drupal\Core\Block\Attribute\Block;
use Drupal\Core\StringTranslation\TranslatableMarkup;

#[Block(
  id: 'my_custom_block',
  admin_label: new TranslatableMarkup('My Custom Block'),
)]
class MyBlock extends BlockBase {

  public function build(): array {
    return [
      '#theme' => 'my_custom_block',
      '#attached' => ['library' => ['my_module/my-block']],
      '#cache' => ['max-age' => 3600],
    ];
  }

}
```

### WordPress：Gutenberg 自定义块（block.json + JS + PHP 渲染）

**block.json**
```json
{
  "$schema": "https://schemas.wp.org/trunk/block.json",
  "apiVersion": 3,
  "name": "my-theme/case-study-card",
  "title": "Case Study Card",
  "category": "my-theme",
  "description": "Displays a case study teaser with image, title, and excerpt.",
  "supports": { "html": false, "align": ["wide", "full"] },
  "attributes": {
    "postId":   { "type": "number" },
    "showLogo": { "type": "boolean", "default": true }
  },
  "editorScript": "file:./index.js",
  "render": "file:./render.php"
}
```

**render.php**
```php
<?php
$post = get_post( $attributes['postId'] ?? 0 );
if ( ! $post ) return;
$show_logo = $attributes['showLogo'] ?? true;
?>
<article <?php echo get_block_wrapper_attributes( [ 'class' => 'case-study-card' ] ); ?>>
    <?php if ( $show_logo && has_post_thumbnail( $post ) ) : ?>
        <div class="case-study-card__image">
            <?php echo get_the_post_thumbnail( $post, 'medium', [ 'loading' => 'lazy' ] ); ?>
        </div>
    <?php endif; ?>
    <div class="case-study-card__body">
        <h3 class="case-study-card__title">
            <a href="<?php echo esc_url( get_permalink( $post ) ); ?>">
                <?php echo esc_html( get_the_title( $post ) ); ?>
            </a>
        </h3>
        <p class="case-study-card__excerpt"><?php echo esc_html( get_the_excerpt( $post ) ); ?></p>
    </div>
</article>
```

### WordPress：自定义 ACF 块（PHP 渲染回调）

```php
// 在 functions.php 或 inc/acf-fields.php 中
add_action( 'acf/init', function () {
    acf_register_block_type( [
        'name'            => 'testimonial',
        'title'           => 'Testimonial',
        'render_callback' => 'my_theme_render_testimonial',
        'category'        => 'my-theme',
        'icon'            => 'format-quote',
        'keywords'        => [ 'quote', 'review' ],
        'supports'        => [ 'align' => false, 'jsx' => true ],
        'example'         => [ 'attributes' => [ 'mode' => 'preview' ] ],
    ] );
} );

function my_theme_render_testimonial( $block ) {
    $quote  = get_field( 'quote' );
    $author = get_field( 'author_name' );
    $role   = get_field( 'author_role' );
    $classes = 'testimonial-block ' . esc_attr( $block['className'] ?? '' );
    ?>
    <blockquote class="<?php echo trim( $classes ); ?>">
        <p class="testimonial-block__quote"><?php echo esc_html( $quote ); ?></p>
        <footer class="testimonial-block__attribution">
            <strong><?php echo esc_html( $author ); ?></strong>
            <?php if ( $role ) : ?><span><?php echo esc_html( $role ); ?></span><?php endif; ?>
        </footer>
    </blockquote>
    <?php
}
```

### WordPress：Enqueue 脚本与样式（正确模式）

```php
add_action( 'wp_enqueue_scripts', function () {
    $theme_ver = wp_get_theme()->get( 'Version' );

    wp_enqueue_style(
        'my-theme-styles',
        get_stylesheet_directory_uri() . '/assets/css/main.css',
        [],
        $theme_ver
    );

    wp_enqueue_script(
        'my-theme-scripts',
        get_stylesheet_directory_uri() . '/assets/js/main.js',
        [],
        $theme_ver,
        [ 'strategy' => 'defer' ]   // WP 6.3+ defer/async 支持
    );

    // 将 PHP 数据传递给 JS
    wp_localize_script( 'my-theme-scripts', 'MyTheme', [
        'ajaxUrl' => admin_url( 'admin-ajax.php' ),
        'nonce'   => wp_create_nonce( 'my-theme-nonce' ),
        'homeUrl' => home_url(),
    ] );
} );
```

### Drupal：带无障碍标记的 Twig 模板

```twig
{# templates/node/node--case-study--teaser.html.twig #}
{%
  set classes = [
    'node',
    'node--type-' ~ node.bundle|clean_class,
    'node--view-mode-' ~ view_mode|clean_class,
    'case-study-card',
  ]
%}

<article{{ attributes.addClass(classes) }}>

  {% if content.field_hero_image %}
    <div class="case-study-card__image" aria-hidden="true">
      {{ content.field_hero_image }}
    </div>
  {% endif %}

  <div class="case-study-card__body">
    <h3 class="case-study-card__title">
      <a href="{{ url }}" rel="bookmark">{{ label }}</a>
    </h3>

    {% if content.body %}
      <div class="case-study-card__excerpt">
        {{ content.body|without('#printed') }}
      </div>
    {% endif %}

    {% if content.field_client_logo %}
      <div class="case-study-card__logo">
        {{ content.field_client_logo }}
      </div>
    {% endif %}
  </div>

</article>
```

### Drupal：主题 .libraries.yml

```yaml
# my_theme.libraries.yml
global:
  version: 1.x
  css:
    theme:
      assets/css/main.css: {}
  js:
    assets/js/main.js: { attributes: { defer: true } }
  dependencies:
    - core/drupal
    - core/once

case-study-card:
  version: 1.x
  css:
    component:
      assets/css/components/case-study-card.css: {}
  dependencies:
    - my_theme/global
```

### Drupal：Preprocess Hook（主题层）

```php
<?php
// my_theme.theme

/**
 * 为 case_study 节点实现 template_preprocess_node()。
 */
function my_theme_preprocess_node__case_study(array &$variables): void {
  $node = $variables['node'];

  // 仅在此模板渲染时附加组件库。
  $variables['#attached']['library'][] = 'my_theme/case-study-card';

  // 为客户名字段暴露一个干净变量。
  if ($node->hasField('field_client_name') && !$node->get('field_client_name')->isEmpty()) {
    $variables['client_name'] = $node->get('field_client_name')->value;
  }

  // 为 SEO 添加结构化数据。
  $variables['#attached']['html_head'][] = [
    [
      '#type'       => 'html_tag',
      '#tag'        => 'script',
      '#value'      => json_encode([
        '@context' => 'https://schema.org',
        '@type'    => 'Article',
        'name'     => $node->getTitle(),
      ]),
      '#attributes' => ['type' => 'application/ld+json'],
    ],
    'case-study-schema',
  ];
}
```

---
