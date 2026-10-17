<script setup>
defineProps({ billing: Object, busy: Boolean, date: Function });
const emit = defineEmits(['buy', 'pay']);
</script>
<template>
  <div class="page-heading">
    <div>
      <div class="eyebrow">SIMPLE & TRANSPARENT</div>
      <h1>每一份投入，都清清楚楚。</h1>
      <p>
        成员独立积分 · 无周期清零 · 仅模拟支付 ·
        {{ billing.plan ? '基础套餐已开通' : '尚未开通套餐' }}
      </p>
    </div>
  </div>
  <div class="stat-grid">
    <div class="panel stat">
      <span>可用积分</span><strong>{{ billing.available }}</strong
      ><small>用于发起新的创作</small>
    </div>
    <div class="panel stat">
      <span>预留积分</span><strong>{{ billing.reserved }}</strong
      ><small>待确认或制作中的任务</small>
    </div>
    <div class="panel stat">
      <span>账户余额</span><strong>{{ billing.balance }}</strong
      ><small>余额 = 可用 + 预留</small>
    </div>
  </div>
  <div class="two-columns product-grid">
    <section class="panel product" v-for="p in billing.products" :key="p.id">
      <span class="eyebrow">{{ p.id === 'basic' ? 'START CREATING' : 'KEEP EXPLORING' }}</span>
      <h2>{{ p.name }}</h2>
      <strong>{{ p.points }} <small>积分</small></strong>
      <p>¥{{ (p.amount_cents / 100).toFixed(2) }} · 演示定价</p>
      <button
        class="primary"
        @click="emit('buy', p.id)"
        :disabled="busy || billing.simulated_payment_enabled === false"
      >
        {{
          billing.simulated_payment_enabled === false
            ? '请联系管理员分配积分'
            : '模拟购买（不真实扣款）'
        }}
      </button>
    </section>
  </div>
  <section class="panel padded">
    <h2>业务账单</h2>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>订单</th>
            <th>积分</th>
            <th>金额</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="o in billing.orders" :key="o.id">
            <td>{{ date(o.created_at) }}</td>
            <td>{{ o.sku === 'basic' ? '基础套餐' : '积分加量包' }}</td>
            <td>+{{ o.points }}</td>
            <td>¥{{ (o.amount_cents / 100).toFixed(2) }}</td>
            <td>
              <button
                v-if="o.status === 'pending'"
                :disabled="busy || billing.simulated_payment_enabled === false"
                @click="emit('pay', o.id)"
              >
                完成模拟支付</button
              ><span v-else>模拟已支付</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <h2 class="spaced">积分明细</h2>
    <div class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>时间</th>
            <th>类型</th>
            <th>变动</th>
            <th>变动后余额</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="l in billing.ledger" :key="l.id">
            <td>{{ date(l.created_at) }}</td>
            <td>
              {{
                {
                  purchase: '模拟购买',
                  settle: '生成结算',
                  release: '预留释放',
                  reserve: '任务预留'
                }[l.kind] || l.kind
              }}
            </td>
            <td>{{ l.delta > 0 ? '+' : '' }}{{ l.delta }}</td>
            <td>{{ l.balance_after }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </section>
</template>
