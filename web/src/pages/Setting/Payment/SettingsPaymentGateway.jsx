/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

import React, { useEffect, useState, useRef } from 'react';
import { Button, Form, Row, Col, Typography, Spin } from '@douyinfe/semi-ui';
const { Text } = Typography;
import {
  API,
  removeTrailingSlash,
  showError,
  showSuccess,
  toBoolean,
  verifyJSON,
} from '../../../helpers';
import { useTranslation } from 'react-i18next';

export default function SettingsPaymentGateway(props) {
  const { t } = useTranslation();
  const [loading, setLoading] = useState(false);
  const [inputs, setInputs] = useState({
    PayAddress: '',
    EpayId: '',
    EpayKey: '',
    Price: 7.3,
    MinTopUp: 1,
    TopupGroupRatio: '',
    CustomCallbackAddress: '',
    PayMethods: '',
    AmountOptions: '',
    AmountDiscount: '',
    // 515pay配置
    Pay515Enabled: false,
    Pay515ApiUrl: '',
    Pay515Pid: '',
    Pay515PlatformPublicKey: '',
    Pay515MerchantPrivateKey: '',
  });
  const [originInputs, setOriginInputs] = useState({});
  const formApiRef = useRef(null);

  useEffect(() => {
    if (props.options && formApiRef.current) {
      const currentInputs = {
        PayAddress: props.options.PayAddress || '',
        EpayId: props.options.EpayId || '',
        EpayKey: props.options.EpayKey || '',
        Price:
          props.options.Price !== undefined
            ? parseFloat(props.options.Price)
            : 7.3,
        MinTopUp:
          props.options.MinTopUp !== undefined
            ? parseFloat(props.options.MinTopUp)
            : 1,
        TopupGroupRatio: props.options.TopupGroupRatio || '',
        CustomCallbackAddress: props.options.CustomCallbackAddress || '',
        PayMethods: props.options.PayMethods || '',
        AmountOptions: props.options.AmountOptions || '',
        AmountDiscount: props.options.AmountDiscount || '',
        // 515pay配置
        Pay515Enabled: toBoolean(props.options.Pay515Enabled),
        Pay515ApiUrl: props.options.Pay515ApiUrl || '',
        Pay515Pid: props.options.Pay515Pid || '',
        Pay515PlatformPublicKey: props.options.Pay515PlatformPublicKey || '',
        Pay515MerchantPrivateKey: props.options.Pay515MerchantPrivateKey || '',
      };

      // 美化 JSON 展示
      try {
        if (currentInputs.AmountOptions) {
          currentInputs.AmountOptions = JSON.stringify(
            JSON.parse(currentInputs.AmountOptions),
            null,
            2,
          );
        }
      } catch {}
      try {
        if (currentInputs.AmountDiscount) {
          currentInputs.AmountDiscount = JSON.stringify(
            JSON.parse(currentInputs.AmountDiscount),
            null,
            2,
          );
        }
      } catch {}

      setInputs(currentInputs);
      setOriginInputs({ ...currentInputs });
      formApiRef.current.setValues(currentInputs);
    }
  }, [props.options]);

  const handleFormChange = (values) => {
    setInputs(values);
  };

  const submitPayAddress = async () => {
    if (props.options.ServerAddress === '') {
      showError(t('请先填写服务器地址'));
      return;
    }

    if (originInputs['TopupGroupRatio'] !== inputs.TopupGroupRatio) {
      if (!verifyJSON(inputs.TopupGroupRatio)) {
        showError(t('充值分组倍率不是合法的 JSON 字符串'));
        return;
      }
    }

    if (originInputs['PayMethods'] !== inputs.PayMethods) {
      if (!verifyJSON(inputs.PayMethods)) {
        showError(t('充值方式设置不是合法的 JSON 字符串'));
        return;
      }
    }

    if (
      originInputs['AmountOptions'] !== inputs.AmountOptions &&
      inputs.AmountOptions.trim() !== ''
    ) {
      if (!verifyJSON(inputs.AmountOptions)) {
        showError(t('自定义充值数量选项不是合法的 JSON 数组'));
        return;
      }
    }

    if (
      originInputs['AmountDiscount'] !== inputs.AmountDiscount &&
      inputs.AmountDiscount.trim() !== ''
    ) {
      if (!verifyJSON(inputs.AmountDiscount)) {
        showError(t('充值金额折扣配置不是合法的 JSON 对象'));
        return;
      }
    }

    setLoading(true);
    try {
      const options = [
        { key: 'PayAddress', value: removeTrailingSlash(inputs.PayAddress) },
      ];

      if (inputs.EpayId !== '') {
        options.push({ key: 'EpayId', value: inputs.EpayId });
      }
      if (inputs.EpayKey !== undefined && inputs.EpayKey !== '') {
        options.push({ key: 'EpayKey', value: inputs.EpayKey });
      }
      if (inputs.Price !== '') {
        options.push({ key: 'Price', value: inputs.Price.toString() });
      }
      if (inputs.MinTopUp !== '') {
        options.push({ key: 'MinTopUp', value: inputs.MinTopUp.toString() });
      }
      if (inputs.CustomCallbackAddress !== '') {
        options.push({
          key: 'CustomCallbackAddress',
          value: inputs.CustomCallbackAddress,
        });
      }
      if (originInputs['TopupGroupRatio'] !== inputs.TopupGroupRatio) {
        options.push({ key: 'TopupGroupRatio', value: inputs.TopupGroupRatio });
      }
      if (originInputs['PayMethods'] !== inputs.PayMethods) {
        options.push({ key: 'PayMethods', value: inputs.PayMethods });
      }
      if (originInputs['AmountOptions'] !== inputs.AmountOptions) {
        options.push({
          key: 'payment_setting.amount_options',
          value: inputs.AmountOptions,
        });
      }
      if (originInputs['AmountDiscount'] !== inputs.AmountDiscount) {
        options.push({
          key: 'payment_setting.amount_discount',
          value: inputs.AmountDiscount,
        });
      }

      // 515pay配置 - 总是保存以确保开关状态正确
      options.push({
        key: 'Pay515Enabled',
        value: inputs.Pay515Enabled ? 'true' : 'false',
      });
      if (inputs.Pay515ApiUrl !== originInputs.Pay515ApiUrl) {
        options.push({
          key: 'Pay515ApiUrl',
          value: removeTrailingSlash(inputs.Pay515ApiUrl),
        });
      }
      if (inputs.Pay515Pid !== originInputs.Pay515Pid) {
        options.push({ key: 'Pay515Pid', value: inputs.Pay515Pid });
      }
      if (inputs.Pay515PlatformPublicKey !== originInputs.Pay515PlatformPublicKey) {
        options.push({ key: 'Pay515PlatformPublicKey', value: inputs.Pay515PlatformPublicKey });
      }
      if (inputs.Pay515MerchantPrivateKey !== originInputs.Pay515MerchantPrivateKey) {
        options.push({ key: 'Pay515MerchantPrivateKey', value: inputs.Pay515MerchantPrivateKey });
      }

      // 发送请求
      const requestQueue = options.map((opt) =>
        API.put('/api/option/', {
          key: opt.key,
          value: opt.value,
        }),
      );

      const results = await Promise.all(requestQueue);

      // 检查所有请求是否成功
      const errorResults = results.filter((res) => !res.data.success);
      if (errorResults.length > 0) {
        errorResults.forEach((res) => {
          showError(res.data.message);
        });
      } else {
        showSuccess(t('更新成功'));
        // 更新本地存储的原始值
        setOriginInputs({ ...inputs });
        props.refresh && props.refresh();
      }
    } catch (error) {
      showError(t('更新失败'));
    }
    setLoading(false);
  };

  return (
    <Spin spinning={loading}>
      <Form
        initValues={inputs}
        onValueChange={handleFormChange}
        getFormApi={(api) => (formApiRef.current = api)}
      >
        <Form.Section text={t('支付设置')}>
          <Text>
            {t(
              '（当前仅支持易支付接口，默认使用上方服务器地址作为回调地址！）',
            )}
          </Text>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='PayAddress'
                label={t('支付地址')}
                placeholder={t('例如：https://yourdomain.com')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='EpayId'
                label={t('易支付商户ID')}
                placeholder={t('例如：0001')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='EpayKey'
                label={t('易支付商户密钥')}
                placeholder={t('敏感信息不会发送到前端显示')}
                type='password'
              />
            </Col>
          </Row>
          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='CustomCallbackAddress'
                label={t('回调地址')}
                placeholder={t('例如：https://yourdomain.com')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='Price'
                precision={2}
                label={t('充值价格（x元/美金）')}
                placeholder={t('例如：7，就是7元/美金')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.InputNumber
                field='MinTopUp'
                label={t('最低充值美元数量')}
                placeholder={t('例如：2，就是最低充值2$')}
              />
            </Col>
          </Row>
          <Form.TextArea
            field='TopupGroupRatio'
            label={t('充值分组倍率')}
            placeholder={t('为一个 JSON 文本，键为组名称，值为倍率')}
            autosize
          />
          <Form.TextArea
            field='PayMethods'
            label={t('充值方式设置')}
            placeholder={t('为一个 JSON 文本')}
            autosize
          />

          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col span={24}>
              <Form.TextArea
                field='AmountOptions'
                label={t('自定义充值数量选项')}
                placeholder={t(
                  '为一个 JSON 数组，例如：[10, 20, 50, 100, 200, 500]',
                )}
                autosize
                extraText={t(
                  '设置用户可选择的充值数量选项，例如：[10, 20, 50, 100, 200, 500]',
                )}
              />
            </Col>
          </Row>

          <Row
            gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }}
            style={{ marginTop: 16 }}
          >
            <Col span={24}>
              <Form.TextArea
                field='AmountDiscount'
                label={t('充值金额折扣配置')}
                placeholder={t(
                  '为一个 JSON 对象，例如：{"100": 0.95, "200": 0.9, "500": 0.85}',
                )}
                autosize
                extraText={t(
                  '设置不同充值金额对应的折扣，键为充值金额，值为折扣率，例如：{"100": 0.95, "200": 0.9, "500": 0.85}',
                )}
              />
            </Col>
          </Row>

          <Button onClick={submitPayAddress}>{t('更新支付设置')}</Button>
        </Form.Section>

        {/* 515pay配置 */}
        <Form.Section text={t('515pay支付设置')}>
          <Text>
            {t('（515pay支付接口，支持RSA签名验签）')}
          </Text>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }} style={{ marginTop: 16 }}>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Switch
                field='Pay515Enabled'
                label={t('启用515pay')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='Pay515ApiUrl'
                label={t('515pay接口地址')}
                placeholder={t('例如：https://pay.515shun.fun')}
              />
            </Col>
            <Col xs={24} sm={24} md={8} lg={8} xl={8}>
              <Form.Input
                field='Pay515Pid'
                label={t('515pay商户ID')}
                placeholder={t('商户ID')}
              />
            </Col>
          </Row>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }} style={{ marginTop: 16 }}>
            <Col span={24}>
              <Form.TextArea
                field='Pay515PlatformPublicKey'
                label={t('515pay平台公钥')}
                placeholder={t('平台RSA公钥')}
                autosize
              />
            </Col>
          </Row>
          <Row gutter={{ xs: 8, sm: 16, md: 24, lg: 24, xl: 24, xxl: 24 }} style={{ marginTop: 16 }}>
            <Col span={24}>
              <Form.TextArea
                field='Pay515MerchantPrivateKey'
                label={t('515pay商户私钥')}
                placeholder={t('商户RSA私钥')}
                autosize
              />
            </Col>
          </Row>
          <Button onClick={submitPayAddress}>{t('更新支付设置')}</Button>
        </Form.Section>
      </Form>
    </Spin>
  );
}
