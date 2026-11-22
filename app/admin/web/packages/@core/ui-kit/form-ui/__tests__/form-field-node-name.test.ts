import type { VueWrapper } from '@vue/test-utils';

import { flushPromises, mount } from '@vue/test-utils';
import { defineComponent, h, nextTick, ref } from 'vue';

import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest';

import { COMPONENT_MAP, setupVbenForm } from '../src/config';
import { useVbenForm } from '../src/use-vben-form';

const wrappers: VueWrapper[] = [];

let capturedTreeSelectKeys: string[] = [];
let capturedTreeSelectNodeName: unknown;
let capturedTreeSelectNameValue: unknown;
let capturedDeclaredName: unknown;

const NativeInputLike = defineComponent({
  name: 'NativeInputLike',
  inheritAttrs: false,
  props: {
    value: { type: String, default: '' },
  },
  emits: ['update:value'],
  setup(props, { attrs, emit }) {
    return () =>
      h('input', {
        ...attrs,
        value: props.value,
        onInput: (event: Event) =>
          emit('update:value', (event.target as HTMLInputElement).value),
      });
  },
});

const TreeSelectLike = defineComponent({
  name: 'TreeSelectLike',
  inheritAttrs: false,
  props: {
    data: { type: Array, default: () => [] },
    value: { type: String, default: '' },
  },
  setup(_props, { attrs }) {
    const wrapperRef = ref<HTMLElement | null>(null);
    return () =>
      h(
        'div',
        {
          ref: wrapperRef,
          class: 'el-select__wrapper',
          onClick: () => {
            const form = wrapperRef.value?.closest('form');
            if (form) {
              const rawNodeName = Reflect.get(form, 'nodeName');
              if (typeof rawNodeName !== 'string') {
                throw new TypeError(
                  'form.nodeName.toLowerCase is not a function',
                );
              }
              rawNodeName.toLowerCase();
            }
          },
        },
        attrs.name ? [h('input', { name: String(attrs.name) })] : undefined,
      );
  },
});

const DeclaredNameComponent = defineComponent({
  name: 'DeclaredNameComponent',
  props: {
    name: { type: String, default: '' },
  },
  setup(props) {
    capturedDeclaredName = props.name;
    return () => h('div', `name:${props.name ?? ''}`);
  },
});

const ProbeTreeSelect = defineComponent({
  name: 'ProbeTreeSelect',
  inheritAttrs: false,
  setup(props, { attrs }) {
    capturedTreeSelectKeys = [...Object.keys(props), ...Object.keys(attrs)];
    capturedTreeSelectNodeName = Reflect.get(attrs, 'nodeName');
    capturedTreeSelectNameValue = Reflect.get(attrs, 'name');
    return () => h(TreeSelectLike, { ...props, ...attrs });
  },
});

beforeAll(() => {
  setupVbenForm({
    config: {},
    rules: {},
  });
});

afterEach(() => {
  for (const wrapper of wrappers.splice(0)) {
    wrapper.unmount();
  }
  capturedTreeSelectKeys = [];
  capturedTreeSelectNodeName = undefined;
  capturedTreeSelectNameValue = undefined;
  capturedDeclaredName = undefined;
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('issue #8214: form field named nodeName crashes TreeSelect render', () => {
  it('renders MRE schema (TreeSelect parentId + Input nodeName) without error', async () => {
    const consoleError = vi
      .spyOn(console, 'error')
      .mockImplementation(() => {});
    const [Form] = useVbenForm({
      schema: [
        {
          component: ProbeTreeSelect,
          componentProps: { data: [] },
          fieldName: 'parentId',
        },
        {
          component: NativeInputLike,
          fieldName: 'nodeName',
        },
      ],
    });

    let renderError: unknown;
    const wrapper = mount(Form, {
      global: {
        config: {
          errorHandler: (error) => {
            renderError = error;
          },
        },
      },
    });
    wrappers.push(wrapper);
    await flushPromises();
    await nextTick();

    expect(renderError).toBeUndefined();
    expect(consoleError).not.toHaveBeenCalled();

    await wrapper.find('.el-select__wrapper').trigger('click');
    await flushPromises();
    await nextTick();
    expect(renderError).toBeUndefined();
    expect(consoleError).not.toHaveBeenCalled();

    expect(wrapper.find('input[name="nodeName"]').exists()).toBe(false);
    expect(capturedTreeSelectKeys).toContain('name');
    expect(capturedTreeSelectNodeName).toBeUndefined();
  });

  it('does not leak a nodeName keyed object into TreeSelect props when field is named nodeName', async () => {
    expect(Reflect.get(document.createElement('form'), 'nodeName')).toBe(
      'FORM',
    );
    const [Form] = useVbenForm({
      schema: [
        {
          component: ProbeTreeSelect,
          componentProps: { data: [] },
          fieldName: 'nodeName',
        },
      ],
    });

    let renderError: unknown;
    const wrapper = mount(Form, {
      global: {
        config: {
          errorHandler: (error) => {
            renderError = error;
          },
        },
      },
    });
    wrappers.push(wrapper);
    await flushPromises();
    await nextTick();

    expect(renderError).toBeUndefined();
    expect(capturedTreeSelectKeys).not.toContain('name');
    expect(capturedTreeSelectNodeName).toBeUndefined();
  });

  it('keeps model binding when modelPropName resolves to name and its value collides', async () => {
    const [Form] = useVbenForm({
      schema: [
        {
          component: ProbeTreeSelect,
          componentProps: { data: [] },
          defaultValue: 'nodeName',
          fieldName: 'nodeName',
          modelPropName: 'name',
        },
      ],
    });

    const wrapper = mount(Form);
    wrappers.push(wrapper);
    await flushPromises();
    await nextTick();

    expect(capturedTreeSelectKeys).toContain('name');
    expect(capturedTreeSelectNameValue).toBe('nodeName');
  });

  it('preserves componentProps.name when the component declares name as a semantic prop', async () => {
    const [Form] = useVbenForm({
      schema: [
        {
          component: DeclaredNameComponent,
          componentProps: { name: 'nodeName' },
          fieldName: 'nodeName',
        },
      ],
    });

    const wrapper = mount(Form);
    wrappers.push(wrapper);
    await flushPromises();
    await nextTick();

    expect(capturedDeclaredName).toBe('nodeName');
  });

  it('resolves string components through componentMap before the declared-prop check', async () => {
    COMPONENT_MAP.NameDeclared = DeclaredNameComponent;
    const [Form] = useVbenForm({
      schema: [
        {
          component: 'NameDeclared',
          componentProps: { name: 'nodeName' },
          fieldName: 'nodeName',
        },
      ],
    });

    const wrapper = mount(Form);
    wrappers.push(wrapper);
    await flushPromises();
    await nextTick();

    expect(capturedDeclaredName).toBe('nodeName');
  });
});
