## 💻 技术交付物
### 结构拆分：并排分区
```php
// 两个相关分区并排放置——垂直滚动减半
Grid::make(2)
    ->schema([
        Section::make('睡眠')
            ->icon('heroicon-o-moon')
            ->schema([
                TimePicker::make('bedtime')->required(),
                TimePicker::make('wake_time')->required(),
                // 用 range 滑块替代单选行：
                TextInput::make('sleep_quality')
                    ->extraInputAttributes(['type' => 'range', 'min' => 1, 'max' => 10, 'step' => 1])
                    ->label('睡眠质量（1–10）')
                    ->default(5),
            ]),
        Section::make('晨间能量')
            ->icon('heroicon-o-bolt')
            ->schema([
                TextInput::make('energy_morning')
                    ->extraInputAttributes(['type' => 'range', 'min' => 1, 'max' => 10, 'step' => 1])
                    ->label('起床后能量（1–10）')
                    ->default(5),
            ]),
    ])
    ->columnSpanFull(),
```

### 基于 Tab 的表单重构
```php
Tabs::make('EnergyLog')
    ->tabs([
        Tabs\Tab::make('概览')
            ->icon('heroicon-o-calendar-days')
            ->schema([
                DatePicker::make('date')->required(),
                // 编辑时的摘要占位符：
                Placeholder::make('summary')
                    ->content(fn ($record) => $record
                        ? "睡眠: {$record->sleep_quality}/10 · 晨间: {$record->energy_morning}/10"
                        : null
                    )
                    ->hiddenOn('create'),
            ]),
        Tabs\Tab::make('睡眠与能量')
            ->icon('heroicon-o-bolt')
            ->schema([/* 睡眠 + 能量分区并排 */]),
        Tabs\Tab::make('营养')
            ->icon('heroicon-o-cake')
            ->schema([/* 食物 repeater */]),
        Tabs\Tab::make('崩溃与备注')
            ->icon('heroicon-o-exclamation-triangle')
            ->schema([/* 崩溃 repeater + 备注 textarea */]),
    ])
    ->columnSpanFull()
    ->persistTabInQueryString(),
```

### 带有意义项标签的 Repeater
```php
Repeater::make('crashes')
    ->schema([
        TimePicker::make('time')->required(),
        Textarea::make('description')->required(),
    ])
    ->itemLabel(fn (array $state): ?string =>
        isset($state['time'], $state['description'])
            ? $state['time'] . ' — ' . \Str::limit($state['description'], 40)
            : null
    )
    ->collapsible()
    ->collapsed()
    ->addActionLabel('添加崩溃时刻'),
```

### 可折叠的次要分区
```php
Section::make('备注')
    ->icon('heroicon-o-pencil')
    ->schema([
        Textarea::make('notes')
            ->placeholder('今日任何备注——药物、天气、心情...')
            ->rows(4),
    ])
    ->collapsible()
    ->collapsed()  // 默认隐藏——大多数天没有备注
    ->columnSpanFull(),
```

### 导航优化
```php
// 在 app/Providers/Filament/AdminPanelProvider.php
public function panel(Panel $panel): Panel
{
    return $panel
        ->navigationGroups([
            NavigationGroup::make('店铺管理')
                ->icon('heroicon-o-shopping-bag'),
            NavigationGroup::make('用户与权限')
                ->icon('heroicon-o-users'),
            NavigationGroup::make('系统')
                ->icon('heroicon-o-cog-6-tooth')
                ->collapsed(),
        ]);
}
```

### 动态条件字段
```php
Forms\Components\Select::make('type')
    ->options(['physical' => '实物', 'digital' => '数字'])
    ->live(),

Forms\Components\TextInput::make('weight')
    ->hidden(fn (Get $get) => $get('type') !== 'physical')
    ->required(fn (Get $get) => $get('type') === 'physical'),
```
