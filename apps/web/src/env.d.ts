/// <reference types="vite/client" />

// echarts-liquidfill 无官方类型声明（UMD 包），仅用于注册 liquidFill 系列，无导出消费
declare module 'echarts-liquidfill';

declare module '*.vue' {
  import type { DefineComponent } from 'vue';
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  const component: DefineComponent<{}, {}, unknown>;
  export default component;
}

// novnc-core 无官方类型声明（CJS 包），按实际用到的 RFB 接口最小面声明
declare module 'novnc-core' {
  interface RfbOptions {
    credentials?: { password?: string; username?: string; target?: string };
    shared?: boolean;
    repeaterID?: string;
    showDotCursor?: boolean;
  }

  export default class RFB {
    constructor(target: HTMLElement, url: string, options?: RfbOptions);
    addEventListener(
      type: 'connect' | 'disconnect' | 'credentialsrequired' | 'desktopname' | 'clipboard' | 'bell' | 'capabilities' | 'securityfailure',
      listener: (e: CustomEvent) => void,
    ): void;
    removeEventListener(type: string, listener: (e: CustomEvent) => void): void;
    disconnect(): void;
    sendCredentials(credentials: { password?: string; username?: string; target?: string }): void;
    sendKey(keysym: number, code: string | null, down?: boolean): void;
    sendCtrlAltDel(): void;
    focus(): void;
    blur(): void;
    clipboardPasteFrom(text: string): void;
    resizeSession: boolean;
    scaleViewport: boolean;
    viewOnly: boolean;
    background: string;
    qualityLevel: number;
    compressionLevel: number;
  }
}
