document.addEventListener('DOMContentLoaded', () => {
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

  document.getElementById('podForm').addEventListener('submit', (event) => {
      event.preventDefault();

      const formData = new FormData(event.target);
      const data = Object.fromEntries(formData.entries());

      console.log('Form Data:', data);

      // TO DO: Bring back to pod home page
      alert('Pod created successfully!');
  });
});