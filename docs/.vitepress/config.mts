import { defineConfig, type DefaultTheme } from 'vitepress'

const repository = 'https://github.com/Tangerg/oolong'

const englishSidebar: DefaultTheme.SidebarItem[] = [
  {
    text: 'Start',
    items: [
      { text: 'Introduction', link: '/' },
      { text: 'Build Your First Interface', link: '/guides/getting-started' },
      { text: 'Choose Modules', link: '/guides/modules' }
    ]
  },
  {
    text: 'Build',
    items: [
      { text: 'Compose Components', link: '/guides/components' },
      { text: 'Render Rich Content', link: '/guides/content' },
      { text: 'Stream Bounded Output', link: '/guides/streaming' },
      { text: 'Build an Agent Interface', link: '/guides/agent' },
      { text: 'History and Virtualization', link: '/guides/history' }
    ]
  },
  {
    text: 'Validate',
    items: [
      { text: 'Test an Interface', link: '/guides/testing' },
      { text: 'Troubleshoot Applications', link: '/guides/troubleshooting' }
    ]
  },
  {
    text: 'Understand',
    items: [
      { text: 'Architecture', link: '/core/architecture' },
      { text: 'Logical Text', link: '/core/text' },
      { text: 'Lifetimes', link: '/core/lifetimes' },
      { text: 'Base Behavior', link: '/base/' },
      { text: 'Kit Appearance', link: '/kit/' },
      { text: 'Ownership Decisions', link: '/decisions/ownership' },
      { text: 'Prior Art', link: '/decisions/prior-art' },
      { text: 'Brand', link: '/decisions/brand' }
    ]
  },
  {
    text: 'Maintain',
    items: [{ text: 'Prepare a Release', link: '/guides/releasing' }]
  }
]

const chineseSidebar: DefaultTheme.SidebarItem[] = [
  {
    text: '开始',
    items: [
      { text: '项目介绍', link: '/zh/' },
      { text: '构建第一个界面', link: '/zh/guides/getting-started' },
      { text: '选择模块', link: '/zh/guides/modules' }
    ]
  },
  {
    text: '构建',
    items: [
      { text: '组合组件', link: '/zh/guides/components' },
      { text: '渲染富文本', link: '/zh/guides/content' },
      { text: '构建有界流式输出', link: '/zh/guides/streaming' },
      { text: '构建 Agent 界面', link: '/zh/guides/agent' },
      { text: '历史与虚拟化', link: '/zh/guides/history' }
    ]
  },
  {
    text: '验证',
    items: [
      { text: '测试界面', link: '/zh/guides/testing' },
      { text: '排查应用问题', link: '/zh/guides/troubleshooting' }
    ]
  },
  {
    text: '理解',
    items: [
      { text: '架构', link: '/zh/core/architecture' },
      { text: '逻辑文本', link: '/zh/core/text' },
      { text: '生命周期', link: '/zh/core/lifetimes' },
      { text: 'Base 行为', link: '/zh/base/' },
      { text: 'Kit 外观', link: '/zh/kit/' },
      { text: '所有权决策', link: '/zh/decisions/ownership' },
      { text: '既有设计', link: '/zh/decisions/prior-art' },
      { text: '品牌', link: '/zh/decisions/brand' }
    ]
  },
  {
    text: '维护',
    items: [{ text: '准备发布', link: '/zh/guides/releasing' }]
  }
]

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
        nav: [
          { text: 'Guide', link: '/guides/getting-started' },
          { text: 'Architecture', link: '/core/architecture' },
          { text: 'Examples', link: '/guides/examples' },
          { text: 'Go Reference', link: 'https://pkg.go.dev/github.com/Tangerg/oolong/ui' }
        ],
        sidebar: englishSidebar,
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
        nav: [
          { text: '指南', link: '/zh/guides/getting-started' },
          { text: '架构', link: '/zh/core/architecture' },
          { text: '示例', link: '/zh/guides/examples' },
          { text: 'Go 参考', link: 'https://pkg.go.dev/github.com/Tangerg/oolong/ui' }
        ],
        sidebar: chineseSidebar,
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
