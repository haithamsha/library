function createElementWithAttrs(tagName, attributes) {
  let tag = document.createElement(tagName);
  Object.entries(attributes).forEach(([attr, value]) => tag.setAttribute(attr, value));
  return tag;
}

customElements.define('google-books', class GoogleBooks extends HTMLElement {
  static get observedAttributes() {
    return ['keyword'];
  }

  connectedCallback() {
    let keyword = this.getAttribute("keyword");
    if(keyword) this.fetchBooks(keyword);
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if( name == 'keyword' ) this.fetchBooks(newValue);
  }

  fetchBooks(keyword) {
    fetch('https://www.googleapis.com/books/v1/volumes?q='+keyword)
      .then(response => response.json())
      .then(data => this.update(data));
  }

  update(data) {
    this.innerHTML = '';

    if(!data.items) return;

    let columns = createElementWithAttrs('div', { class: 'columns' });
    this.appendChild(columns);

    for( let i=0; i < data.items.length; i++ ) {
      let column = createElementWithAttrs('div', { class: 'column' });
      columns.appendChild(column);

      let book = createElementWithAttrs("google-book", {
        book: JSON.stringify(data.items[i])
      });
      column.appendChild(book);
    }
  }
});

customElements.define('google-book', class GoogleBook extends HTMLElement {
  constructor() {
    super();
    this.addEventListener('click', this.clickCallback);
  }
  connectedCallback() {
    let book = this.getAttribute('book');
    book = JSON.parse(book);

    let figure = createElementWithAttrs('figure', {
      class: 'image is-3by4'
    });
    this.appendChild(figure)

    let img = createElementWithAttrs('img', {
      title: book.volumeInfo.title,
      src: `http://books.google.com/books/content?id=${book.id}&printsec=frontcover&img=1&zoom=1&edge=curl&source=gbs_api`
    });
    figure.appendChild(img)
  }

  clickCallback() {
    let book = JSON.parse(this.getAttribute('book'));
    let isbn13 = book.volumeInfo.industryIdentifiers.find( i => i.type == 'ISBN_13').identifier;
    this.setValue('google_books_id', book.id);
    this.setValue('isbn', isbn13);
    this.setValue('title', book.volumeInfo.title);
    this.setValue('author', book.volumeInfo.authors.join(', '));
  }

  setValue(name, value) {
    document.getElementsByName(name).forEach( i => i.value = value );
  }
});
