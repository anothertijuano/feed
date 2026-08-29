project('feed-gtk4', 'c',
  version: '0.1.0',
  meson_version: '>= 0.60.0',
  default_options: [
    'c_std=c11',
    'warning_level=2',
    'buildtype=debugoptimized',
  ],
)

gtk4_dep = dependency('gtk4', version: '>= 4.10')
adwaita_dep = dependency('libadwaita-1', version: '>= 1.4')
soup_dep = dependency('libsoup-3.0')
json_dep = dependency('json-glib-1.0')

sources = files(
  'src/main.c',
  'src/config.c',
  'src/api.c',
  'src/item-common.c',
  'src/feed-window.c',
  'src/feed-view.c',
  'src/saved-view.c',
  'src/subs-view.c',
  'src/settings-view.c',
)

executable('feed-gtk4',
  sources,
  dependencies: [gtk4_dep, adwaita_dep, soup_dep, json_dep],
  install: true,
)

subdir('data')
