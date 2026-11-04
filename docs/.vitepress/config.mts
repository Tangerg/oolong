import { defineConfig, type DefaultTheme } from 'vitepress'

const repository = 'https://github.com/Tangerg/oolong'

type Locale = 'en' | 'zh'
type LocalizedText = Readonly<Record<Locale, string>>
type NavigationLink = Readonly<{ text: LocalizedText; link: string }>
type NavigationGroup = Readonly<{ text: LocalizedText; items: readonly NavigationLink[] }>

const gettingStarted: NavigationLink = {
  text: { en: 'Build Your First Interface', zh: '构建第一个界面' },
  link: '/guides/getting-started'
}
const architecture: NavigationLink = {
  text: { en: 'Architecture', zh: '架构' },
  link: '/core/architecture'
}

const sidebar: readonly NavigationGroup[] = [
  {
    text: { en: 'Start', zh: '开始' },
    items: [
      { text: { en: 'Introduction', zh: '项目介绍' }, link: '/' },
      gettingStarted,
      { text: { en: 'Choose Modules', zh: '选择模块' }, link: '/guides/modules' }
    ]
  },
  {
    text: { en: 'Build', zh: '构建' },
    items: [
      { text: { en: 'Compose Components', zh: '组合组件' }, link: '/guides/components' },
      { text: { en: 'Render Rich Content', zh: '渲染富文本' }, link: '/guides/content' },
      { text: { en: 'Stream Bounded Output', zh: '构建有界流式输出' }, link: '/guides/streaming' },
      { text: { en: 'Build an Agent Interface', zh: '构建 Agent 界面' }, link: '/guides/agent' },
      { text: { en: 'History and Virtualization', zh: '历史与虚拟化' }, link: '/guides/history' }
    ]
  },
  {
    text: { en: 'Validate', zh: '验证' },
    items: [
      { text: { en: 'Test an Interface', zh: '测试界面' }, link: '/guides/testing' },
      { text: { en: 'Troubleshoot Applications', zh: '排查应用问题' }, link: '/guides/troubleshooting' }
    ]
  },
  {
    text: { en: 'Understand', zh: '理解' },
    items: [
      architecture,
      { text: { en: 'Logical Text', zh: '逻辑文本' }, link: '/core/text' },
      { text: { en: 'Lifetimes', zh: '生命周期' }, link: '/core/lifetimes' },
      { text: { en: 'Base Behavior', zh: 'Base 行为' }, link: '/base/' },
      { text: { en: 'Kit Appearance', zh: 'Kit 外观' }, link: '/kit/' },
      { text: { en: 'Ownership Decisions', zh: '所有权决策' }, link: '/decisions/ownership' },
      { text: { en: 'Prior Art', zh: '既有设计' }, link: '/decisions/prior-art' },
      { text: { en: 'Brand', zh: '品牌' }, link: '/decisions/brand' }
    ]
  },
  {
    text: { en: 'Maintain', zh: '维护' },
    items: [{ text: { en: 'Prepare a Release', zh: '准备发布' }, link: '/guides/releasing' }]
  }
]

const nav: readonly NavigationLink[] = [
  { ...gettingStarted, text: { en: 'Guide', zh: '指南' } },
  architecture,
  { text: { en: 'Examples', zh: '示例' }, link: '/guides/examples' },
  { text: { en: 'Go Reference', zh: 'Go 参考' }, link: 'https://pkg.go.dev/github.com/Tangerg/oolong/ui' }
]

function localizeLink(item: NavigationLink, locale: Locale): DefaultTheme.NavItemWithLink {
  return {
    text: item.text[locale],
    link: locale === 'zh' && item.link.startsWith('/') ? `/zh${item.link}` : item.link
  }
}

function localizedNavigation(locale: Locale): Pick<DefaultTheme.Config, 'nav' | 'sidebar'> {
  return {
    nav: nav.map(item => localizeLink(item, locale)),
    sidebar: sidebar.map(group => ({
      text: group.text[locale],
      items: group.items.map(item => localizeLink(item, locale))
    }))
  }
}

const sharedTheme: DefaultTheme.Config = {
  logo: {
    src: '/brand/oolong-icon.png',
    alt: 'Oolong'
  },
  socialLinks: [{ icon: 'github', link: repository }],
  search: {
    provider: 'local',
    options: {
      locales: {
        zh: {
          translations: {
            button: { buttonText: '搜索', buttonAriaLabel: '搜索文档' },
            modal: {
              displayDetails: '显示详细列表',
              resetButtonTitle: '清除查询',
              backButtonTitle: '关闭搜索',
              noResultsText: '没有找到结果',
              footer: {
                selectText: '选择',
                selectKeyAriaLabel: '回车',
                navigateText: '导航',
                navigateUpKeyAriaLabel: '上箭头',
                navigateDownKeyAriaLabel: '下箭头',
                closeText: '关闭',
                closeKeyAriaLabel: 'Esc'
              }
            }
          }
        }
      }
    }
  },
  editLink: {
    pattern: `${repository}/edit/main/docs/:path`,
    text: 'Edit this page on GitHub'
  },
  footer: {
    message: 'Released under the Apache 2.0 License.',
    copyright: 'Copyright © 2025-present Oolong contributors'
  }
}

export default defineConfig({
  base: '/oolong/',
  cleanUrls: true,
  lastUpdated: true,
  sitemap: { hostname: 'https://tangerg.github.io/oolong/' },
  rewrites: {
    'README.md': 'index.md',
    'zh/README.md': 'zh/index.md'
  },
  head: [
    ['link', { rel: 'icon', href: '/oolong/brand/oolong-icon.png', type: 'image/png' }],
    ['meta', { name: 'theme-color', content: '#071127' }]
  ],
  locales: {
    root: {
      label: 'English',
      lang: 'en-US',
      title: 'Oolong',
      description: 'A streaming-first terminal UI substrate for Go.',
      themeConfig: {
        ...sharedTheme,
        ...localizedNavigation('en'),
        outline: { label: 'On this page', level: [2, 3] },
        docFooter: { prev: 'Previous page', next: 'Next page' }
      }
    },
    zh: {
      label: '简体中文',
      lang: 'zh-CN',
      link: '/zh/',
      title: 'Oolong',
      description: '面向 Go 的流式优先终端 UI 底座。',
      themeConfig: {
        ...sharedTheme,
        ...localizedNavigation('zh'),
        outline: { label: '本页内容', level: [2, 3] },
        docFooter: { prev: '上一页', next: '下一页' },
        editLink: {
          pattern: `${repository}/edit/main/docs/:path`,
          text: '在 GitHub 上编辑此页'
        },
        lastUpdated: { text: '最后更新于' },
        darkModeSwitchLabel: '外观',
        sidebarMenuLabel: '菜单',
        returnToTopLabel: '返回顶部',
        langMenuLabel: '切换语言',
        footer: {
          message: '基于 Apache 2.0 许可证发布。',
          copyright: 'Copyright © 2025-present Oolong contributors'
        }
      }
    }
  },
  themeConfig: sharedTheme
})
