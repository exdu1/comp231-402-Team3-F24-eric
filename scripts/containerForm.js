document.addEventListener('DOMContentLoaded', () => {
  const form = document.getElementById('containerForm');
  const useImageRadio = document.getElementById('useImage');
  const noImageRadio = document.getElementById('noImage');
  const imageRow = document.getElementById('imageRow');

  // Function to toggle the visibility of the image input field
  function toggleImageInput() {
      if (useImageRadio.checked) {
          imageRow.style.display = 'table-row';
          document.getElementById('image').required = true;
      } else {
          imageRow.style.display = 'none';
          document.getElementById('image').required = false;
      }
  }

  // Add event listeners to the radio buttons
  useImageRadio.addEventListener('change', toggleImageInput);
  noImageRadio.addEventListener('change', toggleImageInput);

  // Initial call to set the correct visibility
  toggleImageInput();

  // Add environment variable entry
  document.querySelector('.add-env-var').addEventListener('click', (event) => {
      event.preventDefault();
      const envVarsDiv = document.querySelector('.env-vars');
      const newEntry = document.createElement('div');
      newEntry.classList.add('env-var-entry');
      newEntry.innerHTML = `
          <input type="text" name="env_key[]" placeholder="KEY">
          <input type="text" name="env_value[]" placeholder="VALUE">
          <button type="button" class="remove-env-var">-</button>
      `;
      envVarsDiv.appendChild(newEntry);

      // Add event listener to the remove button
      newEntry.querySelector('.remove-env-var').addEventListener('click', (event) => {
          event.preventDefault();
          newEntry.remove();
      });
  });

  form.addEventListener('submit', (event) => {
      event.preventDefault();

      const formData = new FormData(form);
      const data = Object.fromEntries(formData.entries());

      console.log('Form Data:', data);

      // For now, just log the form data
      alert('Container created successfully!');
  });
});