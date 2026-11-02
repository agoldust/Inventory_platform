<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'

interface Product {
  id: number
  name: string
  price: number
  stock: number
}

const products = ref<Product[]>([])
const error = ref('')
// A = table layout, B = card grid.
const variant = ref<'A' | 'B'>('A')
const newProduct = reactive({ name: '', price: 0 })

/** Calls the API; throws an Error carrying the server's message on non-2xx. */
async function api<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch('/api' + path, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  const body = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(body.error || `Request failed (${res.status})`)
  return body as T
}

/** Runs fn, showing any failure in the error banner. */
async function guarded(fn: () => Promise<void>): Promise<void> {
  error.value = ''
  try {
    await fn()
  } catch (e) {
    error.value = e instanceof Error ? e.message : String(e)
  }
}

const load = () => guarded(async () => {
  products.value = await api<Product[]>('/products')
})

/** Creates a product (stock starts at 0; use +/- to add units), then reloads. */
const addProduct = () => guarded(async () => {
  await api('/products', {
    method: 'POST',
    body: JSON.stringify({ name: newProduct.name, price: Number(newProduct.price) }),
  })
  newProduct.name = ''
  newProduct.price = 0
  await load()
})

/** Adds delta (negative to remove) to a product's stock, then reloads. */
const changeStock = (p: Product, delta: number) => guarded(async () => {
  await api(`/products/${p.id}/stock`, {
    method: 'PATCH',
    body: JSON.stringify({ change: delta, reason: 'manual adjustment' }),
  })
  await load()
})

/** Deletes a product after confirmation; its stock history is deleted with it. */
const removeProduct = (p: Product) => guarded(async () => {
  if (!confirm(`Delete "${p.name}" and its stock history?`)) return
  await api(`/products/${p.id}`, { method: 'DELETE' })
  await load()
})

/** Asks the API which A/B layout this browser gets (it sets the cookie on first visit). */
async function loadVariant() {
  try {
    variant.value = (await api<{ variant: 'A' | 'B' }>('/variant')).variant
  } catch {
    // The layout is cosmetic: fall back to A instead of blocking the page.
  }
}

// Variant first, so the product requests that follow already carry the cookie.
onMounted(async () => {
  await loadVariant()
  await load()
})
</script>

<template>
  <main>
    <h1>Inventory</h1>
    <p v-if="error" class="error">{{ error }}</p>

    <form class="row" @submit.prevent="addProduct">
      <input v-model.trim="newProduct.name" placeholder="Name" required />
      <input v-model="newProduct.price" type="number" min="0" step="0.01" placeholder="Price" required />
      <button>Add product</button>
    </form>

    <div v-if="variant === 'B'" class="cards">
      <article v-for="p in products" :key="p.id" class="card">
        <h2>{{ p.name }}</h2>
        <p class="price">{{ p.price.toFixed(2) }}</p>
        <p class="stock">{{ p.stock }} in stock</p>
        <div class="row">
          <button :disabled="p.stock <= 0" aria-label="Decrease stock" @click="changeStock(p, -1)">−</button>
          <button aria-label="Increase stock" @click="changeStock(p, 1)">+</button>
          <button class="danger" @click="removeProduct(p)">Delete</button>
        </div>
      </article>
      <p v-if="!products.length">No products yet.</p>
    </div>

    <table v-else>
      <thead>
        <tr><th>ID</th><th>Name</th><th>Price</th><th>Stock</th><th>Actions</th></tr>
      </thead>
      <tbody>
        <tr v-for="p in products" :key="p.id">
          <td>{{ p.id }}</td>
          <td>{{ p.name }}</td>
          <td>{{ p.price.toFixed(2) }}</td>
          <td>{{ p.stock }}</td>
          <td>
            <div class="row">
              <button :disabled="p.stock <= 0" aria-label="Decrease stock" @click="changeStock(p, -1)">−</button>
              <button aria-label="Increase stock" @click="changeStock(p, 1)">+</button>
              <button class="danger" @click="removeProduct(p)">Delete</button>
            </div>
          </td>
        </tr>
        <tr v-if="!products.length"><td colspan="5">No products yet.</td></tr>
      </tbody>
    </table>
  </main>
</template>

<style>
body { font-family: system-ui, sans-serif; margin: 0; background: #f6f7f9; color: #222; }
main { max-width: 720px; margin: 2rem auto; padding: 0 1rem; }
table { width: 100%; border-collapse: collapse; background: #fff; margin-top: 1rem; }
th, td { text-align: left; padding: .5rem .75rem; border-bottom: 1px solid #e3e5e8; }
.cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 1rem; margin-top: 1rem; }
.card { background: #fff; border-radius: 8px; padding: 1rem; box-shadow: 0 1px 3px rgba(0, 0, 0, .12); }
.card h2 { margin: 0 0 .25rem; font-size: 1.1rem; }
.card p { margin: 0 0 .5rem; }
.card .stock { font-size: 1.4rem; font-weight: 600; }
.row { display: flex; gap: .5rem; align-items: center; }
.row input { flex: 1; min-width: 0; }
button { min-width: 2.25rem; }
.danger { color: #9b1c1c; }
.error { background: #fde8e8; color: #9b1c1c; padding: .5rem .75rem; border-radius: 4px; }
</style>
