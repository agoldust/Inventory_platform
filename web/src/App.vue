<script setup>
import { ref, reactive, onMounted } from 'vue'

const products = ref([])
const error = ref('')
const newProduct = reactive({ name: '', price: 0 })
// Per-product stock form state, keyed by product id.
const adjust = reactive({})

/** Calls the API; throws an Error carrying the server's message on non-2xx. */
async function api(path, options) {
  const res = await fetch('/api' + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error || `Request failed (${res.status})`)
  return body
}

/** Runs fn, showing any failure in the error banner. */
async function guarded(fn) {
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e.message
  }
}

const load = () => guarded(async () => {
  products.value = await api('/products')
  for (const p of products.value) adjust[p.id] ||= { change: 1, reason: '' }
})

const addProduct = () => guarded(async () => {
  await api('/products', { method: 'POST', body: JSON.stringify({ name: newProduct.name, price: Number(newProduct.price) }) })
  newProduct.name = ''
  newProduct.price = 0
  await load()
})

const applyStock = (p) => guarded(async () => {
  const f = adjust[p.id]
  await api(`/products/${p.id}/stock`, {
    method: 'PATCH',
    body: JSON.stringify({ change: Number(f.change), reason: f.reason }),
  })
  delete adjust[p.id]
  await load()
})

onMounted(load)
</script>

<template>
  <main>
    <h1>Inventory</h1>
    <p v-if="error" class="error">{{ error }}</p>

    <form class="row" @submit.prevent="addProduct">
      <input v-model="newProduct.name" placeholder="Name" required />
      <input v-model="newProduct.price" type="number" min="0" step="0.01" required />
      <button>Add product</button>
    </form>

    <table>
      <thead>
        <tr><th>ID</th><th>Name</th><th>Price</th><th>Stock</th><th>Adjust stock</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in products" :key="p.id">
          <td>{{ p.id }}</td>
          <td>{{ p.name }}</td>
          <td>{{ p.price.toFixed(2) }}</td>
          <td>{{ p.stock }}</td>
          <td>
            <form class="row" @submit.prevent="applyStock(p)">
              <input v-model="adjust[p.id].change" type="number" step="1" title="Use a negative number to remove stock" required />
              <input v-model="adjust[p.id].reason" placeholder="Reason" required />
              <button>Apply</button>
            </form>
          </td>
        </tr>
        <tr v-if="!products.length"><td colspan="5">No products yet.</td></tr>
      </tbody>
    </table>
  </main>
</template>

<style>
body { font-family: system-ui, sans-serif; margin: 0; background: #f6f7f9; color: #222; }
main { max-width: 860px; margin: 2rem auto; padding: 0 1rem; }
table { width: 100%; border-collapse: collapse; background: #fff; margin-top: 1rem; }
th, td { text-align: left; padding: .5rem .75rem; border-bottom: 1px solid #e3e5e8; }
.row { display: flex; gap: .5rem; }
.row input[type=number] { width: 6rem; }
.error { background: #fde8e8; color: #9b1c1c; padding: .5rem .75rem; border-radius: 4px; }
</style>
